package codegen

import (
	"fmt"
	"path/filepath"
	"text/template"

	"goa.design/goa.v2/codegen"
	httpdesign "goa.design/goa.v2/http/design"
)

// ClientFiles returns the client HTTP transport files.
func ClientFiles(root *httpdesign.RootExpr) []codegen.File {
	fw := make([]codegen.File, 2*len(root.HTTPServices))
	for i, r := range root.HTTPServices {
		fw[i] = client(r)
	}
	for i, r := range root.HTTPServices {
		fw[i+len(root.HTTPServices)] = clientEncodeDecode(r)
	}
	return fw
}

// client returns the client HTTP transport file
func client(r *httpdesign.ServiceExpr) codegen.File {
	path := filepath.Join(codegen.SnakeCase(r.Name()), "http", "client", "client.go")
	data := HTTPServices.Get(r.Name())
	sections := func(genPkg string) []*codegen.Section {
		title := fmt.Sprintf("%s client HTTP transport", r.Name())
		s := []*codegen.Section{
			codegen.Header(title, "client", []*codegen.ImportSpec{
				{Path: "context"},
				{Path: "fmt"},
				{Path: "io"},
				{Path: "net/http"},
				{Path: "strconv"},
				{Path: "strings"},
				{Path: "goa.design/goa.v2", Name: "goa"},
				{Path: "goa.design/goa.v2/http", Name: "goahttp"},
				{Path: genPkg + "/" + codegen.Goify(r.Name(), false)},
			}),
			{Template: clientStructTmpl(r), Data: data},
			{Template: clientInitTmpl(r), Data: data},
		}
		for _, e := range data.Endpoints {
			s = append(s, &codegen.Section{Template: endpointInitTmpl(r), Data: e})
		}

		return s
	}

	return codegen.NewSource(path, sections)
}

// clientEncodeDecode returns the file containing the HTTP client encoding and
// decoding logic.
func clientEncodeDecode(svc *httpdesign.ServiceExpr) codegen.File {
	path := filepath.Join(codegen.SnakeCase(svc.Name()), "http", "client", "encode_decode.go")
	data := HTTPServices.Get(svc.Name())
	sections := func(genPkg string) []*codegen.Section {
		title := fmt.Sprintf("%s HTTP client encoders and decoders", svc.Name())
		s := []*codegen.Section{
			codegen.Header(title, "client", []*codegen.ImportSpec{
				{Path: "fmt"},
				{Path: "io"},
				{Path: "io/ioutil"},
				{Path: "net/http"},
				{Path: "net/url"},
				{Path: "strconv"},
				{Path: "strings"},
				{Path: "goa.design/goa.v2", Name: "goa"},
				{Path: "goa.design/goa.v2/http", Name: "goahttp"},
				{Path: genPkg + "/" + codegen.Goify(svc.Name(), false)},
			}),
		}

		for _, e := range data.Endpoints {
			s = append(s, &codegen.Section{Template: requestEncoderTmpl(svc), Data: e})
			if e.Result != nil || len(e.Errors) > 0 {
				s = append(s, &codegen.Section{Template: responseDecoderTmpl(svc), Data: e})
			}
		}
		return s
	}

	return codegen.NewSource(path, sections)
}

func clientStructTmpl(r *httpdesign.ServiceExpr) *template.Template {
	return template.Must(transTmpl(r).New("client-struct").Parse(clientStructT))
}

func clientInitTmpl(r *httpdesign.ServiceExpr) *template.Template {
	return template.Must(transTmpl(r).New("client-constructor").Parse(clientInitT))
}

func endpointInitTmpl(r *httpdesign.ServiceExpr) *template.Template {
	return template.Must(transTmpl(r).New("client-endpoint").Parse(endpointInitT))
}

func requestEncoderTmpl(r *httpdesign.ServiceExpr) *template.Template {
	return template.Must(transTmpl(r).New("request-encoder").Parse(requestEncoderT))
}

func responseDecoderTmpl(r *httpdesign.ServiceExpr) *template.Template {
	return template.Must(transTmpl(r).New("response-decoder").Parse(responseDecoderT))
}

// input: ServiceData
const clientStructT = `{{ printf "%s lists the %s service endpoint HTTP clients." .ClientStruct .Service.Name | comment }}
type {{ .ClientStruct }} struct {
	{{- range .Endpoints }}
	{{ .Method.VarName }}Doer goahttp.Doer
	{{- end }}
	scheme     string
	host       string
	encoder    func(*http.Request) goahttp.Encoder
	decoder    func(*http.Response) goahttp.Decoder
}
`

// input: ServiceData
const clientInitT = `{{ printf "New%s instantiates HTTP clients for all the %s service servers." .ClientStruct .Service.Name | comment }}
func New{{ .ClientStruct }}(
	scheme string,
	host string,
	doer goahttp.Doer,
	enc func(*http.Request) goahttp.Encoder,
	dec func(*http.Response) goahttp.Decoder,
) *{{ .ClientStruct }} {
	return &{{ .ClientStruct }}{
		{{- range .Endpoints }}
		{{ .Method.VarName }}Doer: doer,
		{{- end }}
		scheme:  scheme,
		host:    host,
		decoder: dec,
		encoder: enc,
	}
}
`

// input: EndpointData
const endpointInitT = `{{ printf "%s returns a endpoint that makes HTTP requests to the %s service %s server." .EndpointInit .ServiceName .Method.Name | comment }}
func (c *{{ .ClientStruct }}) {{ .EndpointInit }}() goa.Endpoint {
	var (
		encodeRequest  = c.{{ .RequestEncoder }}(c.encoder)
		decodeResponse = c.{{ .ResponseDecoder }}(c.decoder)
	)
	return func(ctx context.Context, v interface{}) (interface{}, error) {
		req, err := encodeRequest(v)
		if err != nil {
			return nil, err
		}

		resp, err := c.{{ .Method.VarName }}Doer.Do(req)

		if err != nil {
			return nil, goahttp.ErrRequestError("{{ .ServiceName }}", "{{ .Method.Name }}", err)
		}
		return decodeResponse(resp)
	}
}
`

// input: EndpointData
const requestEncoderT = `{{ printf "%s returns an encoder for requests sent to the %s %s server." .RequestEncoder .ServiceName .Method.Name | comment }}
func (c *{{ .ClientStruct }}) {{ .RequestEncoder }}(encoder func(*http.Request) goahttp.Encoder) func(interface{}) (*http.Request, error) {
	return func(v interface{}) (*http.Request, error) {
	{{- if .Payload.Ref }}
		p, ok := v.({{ .Payload.Ref }})
		if !ok {
			return nil, goahttp.ErrInvalidType("{{ .ServiceName }}", "{{ .Method.Name }}", "{{ .Payload.Ref }}", v)
		}
	{{- end }}

	{{- with (index .Routes 0) }}
		// Build request
		{{- range $i, $arg := .PathInit.Args }}
		var {{ .Name }} {{ .TypeRef }}
			{{ if .Pointer -}}
		if p.{{ .FieldName }} != nil {
			{{- end }}
			{{- .Name }} = {{ if .Pointer }}*{{ end }}p.{{ .FieldName }}
			{{- if .Pointer }}
		}
			{{- end }}
		{{- end }}
		u := &url.URL{Scheme: c.scheme, Host: c.host, Path: {{ .PathInit.Name }}({{ range .PathInit.Args }}{{ .Ref }}, {{ end }})}
		req, err := http.NewRequest("{{ .Verb }}", u.String(), nil)
		if err != nil {
			return nil, goahttp.ErrInvalidURL("{{ $.ServiceName }}", "{{ $.Method.Name }}", u.String(), err)
		}
	{{- end }}

	{{- if .Payload.Ref }}
	{{- if .Payload.Request.ClientBody }}
		body := {{ .Payload.Request.ClientBody.Init.Name }}({{ range .Payload.Request.ClientBody.Init.Args }}{{ if .Pointer }}&{{ end }}{{ .Name }}, {{ end }})
		err = encoder(req).Encode(&body)
		if err != nil {
			return nil, goahttp.ErrEncodingError("{{ .ServiceName }}", "{{ .Method.Name }}", err)
		}
	{{- end }}
	{{- range .Payload.Request.Headers }}
		req.Header.Set("{{ .Name }}", p.{{ .FieldName }})
	{{- end }}
	{{- end }}

		return req, nil
	}
}
`

// input: EndpointData
const responseDecoderT = `{{ printf "%s returns a decoder for responses returned by the %s %s endpoint." .ResponseDecoder .ServiceName .Method.Name | comment }}
func (c *{{ .ClientStruct }}) {{ .ResponseDecoder }}(decoder func(*http.Response) goahttp.Decoder) func(*http.Response) (interface{}, error) {
	return func(resp *http.Response) (interface{}, error) {
		defer resp.Body.Close()
		switch resp.StatusCode {
	{{- range .Result.Responses }}
` + singleResponseT + `
		{{- if .ResultInit }}
			return {{ .ResultInit.Name }}({{ range .ResultInit.Args }}{{ .Ref }},{{ end }}), nil
		{{- else if .ClientBody }}
			return body, nil
		{{- else }}
			return nil, nil
		{{- end }}
	{{- end }}
	{{- range .Errors }}
		{{- with .Response }}
` + singleResponseT + `
		{{- if .ResultInit }}
			return {{ .ResultInit.Name }}({{ range .ResultInit.Args }}{{ .Ref }},{{ end }}), nil
		{{- else if .ClientBody }}
			return body, nil
		{{- else }}
			return nil, nil
		{{- end }}
		{{- end }}
	{{- end }}
		default:
			body, _ := ioutil.ReadAll(resp.Body)
			return nil, goahttp.ErrInvalidResponse("account", "create", resp.StatusCode, string(body))
		}
	}
}
`

const singleResponseT = `case {{ .StatusCode }}:
	{{- if .ClientBody }}
			var (
				body {{ .ClientBody.VarName }}
				err error
			)
			err = decoder(resp).Decode(&body)
			if err != nil {
				return nil, goahttp.ErrDecodingError("{{ $.ServiceName }}", "{{ $.Method.Name }}", err)
			}
		{{- if .ClientBody.ValidateRef }}
			{{ .ClientBody.ValidateRef }}
		{{- end }}
	{{ end }}

	{{- if .Headers }}
			var (
		{{- range .Headers }}
				{{ .VarName }} {{ .TypeRef }}
		{{- end }}
		{{- if not .ClientBody }}
			{{- if .MustValidate }}
				err error
			{{- end }}
		{{- end }}
			)
		{{- range .Headers }}

		{{- if and (or (eq .Type.Name "string") (eq .Type.Name "any")) .Required }}
			{{ .VarName }} = resp.Header.Get("{{ .Name }}")
			if {{ .VarName }} != "" {
				err = goa.MergeErrors(err, goa.MissingFieldError("{{ .Name }}", "header"))
			}

		{{- else if (or (eq .Type.Name "string") (eq .Type.Name "any")) }}
			{{ .VarName }}Raw := resp.Header.Get("{{ .Name }}")
			if {{ .VarName }}Raw != "" {
				{{ .VarName }} = {{ if and (eq .Type.Name "string") .Pointer }}&{{ end }}{{ .VarName }}Raw
			}
			{{- if .DefaultValue }} else {
				{{ .VarName }} = {{ if eq .Type.Name "string" }}{{ printf "%q" .DefaultValue }}{{ else }}{{ printf "%#v" .DefaultValue }}{{ end }}
			}
			{{- end }}

		{{- else if .StringSlice }}
			{{ .VarName }} = resp.Header["{{ .CanonicalName }}"]
			{{ if .Required }}
			if {{ .VarName }} == nil {
				err = goa.MergeErrors(err, goa.MissingFieldError("{{ .Name }}", "header"))
			}
			{{- else if .DefaultValue }}
			if {{ .VarName }} == nil {
				{{ .VarName }} = {{ printf "%#v" .DefaultValue }}
			}
			{{- end }}

		{{- else if .Slice }}
			{{ .VarName }}Raw := resp.Header["{{ .CanonicalName }}"]
				{{ if .Required }} if {{ .VarName }}Raw == nil {
				return nil, goa.MissingFieldError("{{ .Name }}", "header")
			}
			{{- else if .DefaultValue }}
			if {{ .VarName }}Raw == nil {
				{{ .VarName }} = {{ printf "%#v" .DefaultValue }}
			}
			{{- end }}

			{{- if .DefaultValue }}else {
			{{- else if not .Required }}
			if {{ .VarName }}Raw != nil {
			{{- end }}
				{{- template "slice_conversion" . }}
			{{- if or .DefaultValue (not .Required) }}
			}
			{{- end }}

		{{- else }}{{/* not string, not any and not slice */}}
			{{ .VarName }}Raw := resp.Header.Get("{{ .Name }}")
			{{- if .Required }}
			if {{ .VarName }}Raw == "" {
				return nil, goa.MissingFieldError("{{ .Name }}", "header")
			}
			{{- else if .DefaultValue }}
			if {{ .VarName }}Raw == "" {
				{{ .VarName }} = {{ printf "%#v" .DefaultValue }}
			}
			{{- end }}

			{{- if .DefaultValue }}else {
				{{- else if not .Required }}
			if {{ .VarName }}Raw != "" {
			{{- end }}
				{{- template "type_conversion" . }}
			{{- if or .DefaultValue (not .Required) }}
			}
			{{- end }}
		{{- end }}
		{{- if .Validate }}
			{{ .Validate }}
		{{- end }}
		{{- end }}{{/* range .Headers */}}
	{{- end }}

	{{- if .MustValidate }}
			if err != nil {
				return nil, err
			}
	{{- end }}
`