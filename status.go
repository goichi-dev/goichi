package goichi

import (
	"github.com/goichi-dev/goichi/core"
)

// HTTP status codes, re-exported from the core package so applications only
// need to import goichi.
const (
	// 2xx Success
	StatusOK                   = core.StatusOK
	StatusCreated              = core.StatusCreated
	StatusAccepted             = core.StatusAccepted
	StatusNonAuthoritativeInfo = core.StatusNonAuthoritativeInfo
	StatusNoContent            = core.StatusNoContent
	StatusResetContent         = core.StatusResetContent
	StatusPartialContent       = core.StatusPartialContent

	// 3xx Redirection
	StatusMultipleChoices   = core.StatusMultipleChoices
	StatusMovedPermanently  = core.StatusMovedPermanently
	StatusFound             = core.StatusFound
	StatusSeeOther          = core.StatusSeeOther
	StatusNotModified       = core.StatusNotModified
	StatusTemporaryRedirect = core.StatusTemporaryRedirect
	StatusPermanentRedirect = core.StatusPermanentRedirect

	// 4xx Client errors
	StatusBadRequest                   = core.StatusBadRequest
	StatusUnauthorized                 = core.StatusUnauthorized
	StatusPaymentRequired              = core.StatusPaymentRequired
	StatusForbidden                    = core.StatusForbidden
	StatusNotFound                     = core.StatusNotFound
	StatusMethodNotAllowed             = core.StatusMethodNotAllowed
	StatusNotAcceptable                = core.StatusNotAcceptable
	StatusProxyAuthRequired            = core.StatusProxyAuthRequired
	StatusRequestTimeout               = core.StatusRequestTimeout
	StatusConflict                     = core.StatusConflict
	StatusGone                         = core.StatusGone
	StatusLengthRequired               = core.StatusLengthRequired
	StatusPreconditionFailed           = core.StatusPreconditionFailed
	StatusRequestEntityTooLarge        = core.StatusRequestEntityTooLarge
	StatusRequestURITooLong            = core.StatusRequestURITooLong
	StatusUnsupportedMediaType         = core.StatusUnsupportedMediaType
	StatusRequestedRangeNotSatisfiable = core.StatusRequestedRangeNotSatisfiable
	StatusExpectationFailed            = core.StatusExpectationFailed
	StatusTeapot                       = core.StatusTeapot
	StatusUnprocessableEntity          = core.StatusUnprocessableEntity
	StatusLocked                       = core.StatusLocked
	StatusFailedDependency             = core.StatusFailedDependency
	StatusUpgradeRequired              = core.StatusUpgradeRequired
	StatusPreconditionRequired         = core.StatusPreconditionRequired
	StatusTooManyRequests              = core.StatusTooManyRequests
	StatusHeaderFieldsTooLarge         = core.StatusHeaderFieldsTooLarge
	StatusUnavailableForLegalReasons   = core.StatusUnavailableForLegalReasons

	// 5xx Server errors
	StatusInternalServerError           = core.StatusInternalServerError
	StatusNotImplemented                = core.StatusNotImplemented
	StatusBadGateway                    = core.StatusBadGateway
	StatusServiceUnavailable            = core.StatusServiceUnavailable
	StatusGatewayTimeout                = core.StatusGatewayTimeout
	StatusHTTPVersionNotSupported       = core.StatusHTTPVersionNotSupported
	StatusVariantAlsoNegotiates         = core.StatusVariantAlsoNegotiates
	StatusInsufficientStorage           = core.StatusInsufficientStorage
	StatusLoopDetected                  = core.StatusLoopDetected
	StatusNotExtended                   = core.StatusNotExtended
	StatusNetworkAuthenticationRequired = core.StatusNetworkAuthenticationRequired
)
