package core

import (
	"context"
	"fmt"
	"github.com/goccy/go-json"
	"github.com/goichi-dev/goichi/utils"
	"github.com/valyala/fasthttp"
	"mime/multipart"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// Handler responds to a request. Returning a non-nil error hands control to the
// router's error handler.
type Handler func(c *Context) error

// ErrorHandler turns an error returned by a Handler into a response.
type ErrorHandler func(c *Context, err error) error

// Middleware wraps a Handler to run code before and after it.
type Middleware func(Handler) Handler

// Context carries one request and its response. It is created per request and
// must not be retained after the handler returns.
type Context struct {
	RequestCtx *fasthttp.RequestCtx
	Params     map[string]string
	JWTClaims  map[string]any
	Locals     map[string]any
}

// Set stores a value for the lifetime of the request.
func (c *Context) Set(key string, val any) {
	if c.Locals == nil {
		c.Locals = make(map[string]any)
	}
	c.Locals[key] = val
}

// Get returns a value stored by Set, or nil.
func (c *Context) Get(key string) any {
	if c.Locals == nil {
		return nil
	}
	return c.Locals[key]
}

// SetJWT records the verified claims of the request's token.
func (c *Context) SetJWT(claims map[string]any) {
	c.JWTClaims = claims
}

// JWT returns the verified claims of the request's token, or nil when the route
// is unauthenticated.
func (c *Context) JWT() map[string]any {
	return c.JWTClaims
}

// Status sets the response status code and returns c for chaining.
func (c *Context) Status(code int) *Context {
	c.RequestCtx.SetStatusCode(code)
	return c
}

// JSON writes v as an application/json response body.
func (c *Context) JSON(v any) error {
	c.RequestCtx.SetContentType("application/json")
	return json.NewEncoder(c.RequestCtx).Encode(v)
}

// WriteJSONError writes a properly-escaped {"error": msg} body to the response.
// Using json.Marshal (rather than string concatenation) prevents response
// corruption / injection when the message contains quotes or newlines.
func WriteJSONError(ctx *fasthttp.RequestCtx, msg string) error {
	ctx.SetContentType("application/json")
	b, err := json.Marshal(map[string]string{"error": msg})
	if err != nil {
		ctx.SetBodyString(`{"error":"internal server error"}`)
		return err
	}
	ctx.SetBody(b)
	return nil
}

// Param returns the value of a ":name" path parameter.
func (c *Context) Param(key string) string {
	return c.Params[key]
}

// Query returns a URL query parameter, or the empty string.
func (c *Context) Query(key string) string {
	return string(c.RequestCtx.QueryArgs().Peek(key))
}

// QueryDefault returns a URL query parameter, or fallback when it is absent.
func (c *Context) QueryDefault(key, fallback string) string {
	if v := c.Query(key); v != "" {
		return v
	}
	return fallback
}

// QueryInt returns a URL query parameter as an int, or def when it is absent or
// not a number.
func (c *Context) QueryInt(key string, def int) int {
	return utils.ToInt(c.Query(key), def)
}

// ParamInt returns a path parameter as an int, or def when it is absent or not
// a number.
func (c *Context) ParamInt(key string, def int) int {
	return utils.ToInt(c.Param(key), def)
}

// Header returns a request header value.
func (c *Context) Header(key string) string {
	return string(c.RequestCtx.Request.Header.Peek(key))
}

// FormFile returns an uploaded file from a multipart form.
func (c *Context) FormFile(key string) (*multipart.FileHeader, error) {
	return c.RequestCtx.FormFile(key)
}

// SaveFile writes an uploaded file to path.
func (c *Context) SaveFile(file *multipart.FileHeader, path string) error {
	return fasthttp.SaveMultipartFile(file, path)
}

// Context returns the request's context, for cancellation and deadlines.
func (c *Context) Context() context.Context {
	return c.RequestCtx
}

// Ok writes data as JSON with status 200.
func (c *Context) Ok(data any) error {
	return c.Status(StatusOK).JSON(data)
}

// Created writes data as JSON with status 201.
func (c *Context) Created(data any) error {
	return c.Status(StatusCreated).JSON(data)
}

// NoContent responds with status 204 and an empty body.
func (c *Context) NoContent() error {
	c.RequestCtx.SetStatusCode(StatusNoContent)
	return nil
}

func (c *Context) BadRequest(msg string) error {
	return c.Status(StatusBadRequest).JSON(map[string]string{"error": msg})
}

func (c *Context) Unauthorized(msg string) error {
	return c.Status(StatusUnauthorized).JSON(map[string]string{"error": msg})
}

func (c *Context) Forbidden(msg string) error {
	return c.Status(StatusForbidden).JSON(map[string]string{"error": msg})
}

func (c *Context) NotFound(msg string) error {
	return c.Status(StatusNotFound).JSON(map[string]string{"error": msg})
}

func (c *Context) UnprocessableEntity(msg string) error {
	return c.Status(StatusUnprocessableEntity).JSON(map[string]string{"error": msg})
}

func (c *Context) TooManyRequests(msg string) error {
	return c.Status(StatusTooManyRequests).JSON(map[string]string{"error": msg})
}

func (c *Context) InternalError(msg string) error {
	return c.Status(StatusInternalServerError).JSON(map[string]string{"error": msg})
}

const (
	// 2xx Success
	StatusOK                   = 200
	StatusCreated              = 201
	StatusAccepted             = 202
	StatusNonAuthoritativeInfo = 203
	StatusNoContent            = 204
	StatusResetContent         = 205
	StatusPartialContent       = 206

	// 3xx Redirection
	StatusMultipleChoices   = 300
	StatusMovedPermanently  = 301
	StatusFound             = 302
	StatusSeeOther          = 303
	StatusNotModified       = 304
	StatusTemporaryRedirect = 307
	StatusPermanentRedirect = 308

	// 4xx Client errors
	StatusBadRequest                   = 400
	StatusUnauthorized                 = 401
	StatusPaymentRequired              = 402
	StatusForbidden                    = 403
	StatusNotFound                     = 404
	StatusMethodNotAllowed             = 405
	StatusNotAcceptable                = 406
	StatusProxyAuthRequired            = 407
	StatusRequestTimeout               = 408
	StatusConflict                     = 409
	StatusGone                         = 410
	StatusLengthRequired               = 411
	StatusPreconditionFailed           = 412
	StatusRequestEntityTooLarge        = 413
	StatusRequestURITooLong            = 414
	StatusUnsupportedMediaType         = 415
	StatusRequestedRangeNotSatisfiable = 416
	StatusExpectationFailed            = 417
	StatusTeapot                       = 418
	StatusUnprocessableEntity          = 422
	StatusLocked                       = 423
	StatusFailedDependency             = 424
	StatusUpgradeRequired              = 426
	StatusPreconditionRequired         = 428
	StatusTooManyRequests              = 429
	StatusHeaderFieldsTooLarge         = 431
	StatusUnavailableForLegalReasons   = 451

	// 5xx Server errors
	StatusInternalServerError           = 500
	StatusNotImplemented                = 501
	StatusBadGateway                    = 502
	StatusServiceUnavailable            = 503
	StatusGatewayTimeout                = 504
	StatusHTTPVersionNotSupported       = 505
	StatusVariantAlsoNegotiates         = 506
	StatusInsufficientStorage           = 507
	StatusLoopDetected                  = 508
	StatusNotExtended                   = 510
	StatusNetworkAuthenticationRequired = 511
)

// Bind fills v from the request: the JSON body first, then any fields tagged
// query, param or header. If v implements SetDefaults() it is called next, and
// the result is validated against the struct's validate tags.
func (c *Context) Bind(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("bind: v must be a non-nil pointer")
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("bind: v must be a pointer to struct")
	}

	body := c.RequestCtx.PostBody()
	if len(body) > 0 {
		if err := json.Unmarshal(body, v); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
	}

	if err := c.bindRecursive(rv); err != nil {
		return err
	}

	if p, ok := v.(interface{ SetDefaults() }); ok {
		p.SetDefaults()
	} else if p, ok := rv.Addr().Interface().(interface{ SetDefaults() }); ok {
		p.SetDefaults()
	}

	return Validate(v)
}

func (c *Context) bindRecursive(rv reflect.Value) error {
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fv := rv.Field(i)

		if !fv.CanSet() {
			continue
		}

		if field.Anonymous && fv.Kind() == reflect.Struct {
			if err := c.bindRecursive(fv); err != nil {
				return err
			}
			continue
		}

		var raw string
		var found bool

		if key := field.Tag.Get("query"); key != "" {
			raw = c.Query(key)
			found = raw != ""
		} else if key := field.Tag.Get("param"); key != "" {
			raw = c.Param(key)
			found = raw != ""
		} else if key := field.Tag.Get("header"); key != "" {
			raw = c.Header(key)
			found = raw != ""
		}

		if found {
			if err := setField(fv, raw); err != nil {
				name := field.Tag.Get("json")
				if name == "" {
					name = field.Name
				}
				return fmt.Errorf("field %s: %w", name, err)
			}
		}
	}
	return nil
}

func setField(fv reflect.Value, raw string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("expected integer, got %q", raw)
		}
		fv.SetInt(n)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("expected unsigned integer, got %q", raw)
		}
		fv.SetUint(n)

	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return fmt.Errorf("expected number, got %q", raw)
		}
		fv.SetFloat(n)

	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("expected boolean, got %q", raw)
		}
		fv.SetBool(b)
	}
	return nil
}

// Validate checks v against the validate tags on its fields, returning a
// *ValidationError listing every failure.
func Validate(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	var errs []string
	validateStruct(rv, &errs)

	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

func validateStruct(rv reflect.Value, errs *[]string) {
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fv := rv.Field(i)

		if field.Anonymous && fv.Kind() == reflect.Struct {
			validateStruct(fv, errs)
			continue
		}

		tag := field.Tag.Get("validate")
		if tag == "" {
			continue
		}

		name := field.Tag.Get("json")
		if name == "" {
			name = field.Tag.Get("query")
		}
		if name == "" {
			name = field.Tag.Get("param")
		}
		if name == "" {
			name = field.Tag.Get("header")
		}
		if name == "" {
			name = field.Name
		}
		name = strings.Split(name, ",")[0]

		for _, rule := range strings.Split(tag, ",") {
			rule = strings.TrimSpace(rule)
			switch {
			case rule == "required":
				if isZero(fv) {
					*errs = append(*errs, fmt.Sprintf("%s is required", name))
				}
			case rule == "email":
				if fv.Kind() == reflect.String && !isEmail(fv.String()) {
					*errs = append(*errs, fmt.Sprintf("%s must be a valid email", name))
				}
			case rule == "numeric":
				if fv.Kind() == reflect.String && !isNumeric(fv.String()) {
					*errs = append(*errs, fmt.Sprintf("%s must be numeric", name))
				}
			case rule == "alpha":
				if fv.Kind() == reflect.String && !isAlpha(fv.String()) {
					*errs = append(*errs, fmt.Sprintf("%s must contain only letters", name))
				}
			case strings.HasPrefix(rule, "min="):
				if err := checkMin(name, fv, parseIntRule(rule[4:])); err != "" {
					*errs = append(*errs, err)
				}
			case strings.HasPrefix(rule, "max="):
				if err := checkMax(name, fv, parseIntRule(rule[4:])); err != "" {
					*errs = append(*errs, err)
				}
			}
		}
	}
}

func isZero(fv reflect.Value) bool {
	return fv.IsZero()
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

func isEmail(s string) bool {
	if s == "" {
		return true
	}
	if len(s) > 254 {
		return false
	}
	return emailRegex.MatchString(s)
}

func isNumeric(s string) bool {
	if s == "" {
		return true
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func isAlpha(s string) bool {
	if s == "" {
		return true
	}
	for _, ch := range s {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') {
			return false
		}
	}
	return true
}

// ValidationError reports the fields that failed validation.
type ValidationError struct {
	Fields []string
}

func (e *ValidationError) Error() string {
	return "validation failed: " + strings.Join(e.Fields, "; ")
}

func parseIntRule(s string) int64 {
	var n int64
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int64(ch-'0')
		}
	}
	return n
}

func checkMin(name string, fv reflect.Value, min int64) string {
	switch fv.Kind() {
	case reflect.String:
		if int64(len(fv.String())) < min {
			return fmt.Sprintf("%s must be at least %d characters", name, min)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if fv.Int() < min {
			return fmt.Sprintf("%s must be >= %d", name, min)
		}
	case reflect.Float32, reflect.Float64:
		if int64(fv.Float()) < min {
			return fmt.Sprintf("%s must be >= %d", name, min)
		}
	case reflect.Slice, reflect.Array, reflect.Map:
		if int64(fv.Len()) < min {
			return fmt.Sprintf("%s must have at least %d items", name, min)
		}
	}
	return ""
}

func checkMax(name string, fv reflect.Value, max int64) string {
	switch fv.Kind() {
	case reflect.String:
		if int64(len(fv.String())) > max {
			return fmt.Sprintf("%s must be at most %d characters", name, max)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if fv.Int() > max {
			return fmt.Sprintf("%s must be <= %d", name, max)
		}
	case reflect.Float32, reflect.Float64:
		if int64(fv.Float()) > max {
			return fmt.Sprintf("%s must be <= %d", name, max)
		}
	case reflect.Slice, reflect.Array, reflect.Map:
		if int64(fv.Len()) > max {
			return fmt.Sprintf("%s must have at most %d items", name, max)
		}
	}
	return ""
}

// Pagination is an embeddable page/per_page query binding with sane defaults.
type Pagination struct {
	Page    int `query:"page" json:"page" validate:"min=1"`
	PerPage int `query:"per_page" json:"per_page" validate:"min=1"`
}

// SetDefaults applies page 1 and 10 items per page when unset.
func (p *Pagination) SetDefaults() {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PerPage <= 0 {
		p.PerPage = 10
	}
}
