package goichi

import (
	"github.com/goichi-dev/goichi/core"
)

type Pagination = core.Pagination
type ValidationError = core.ValidationError

func Send(c *core.Context, data any) error {
	return c.Ok(data)
}

func SendCreated(c *core.Context, data any) error {
	return c.Created(data)
}

func SendPage(c *core.Context, items any, page, perPage, total int) error {
	return c.Ok(map[string]any{
		"items": items,
		"pagination": map[string]any{
			"page":       page,
			"per_page":   perPage,
			"total":      total,
			"total_page": (total + perPage - 1) / perPage,
		},
	})
}

func ErrBadRequest(c *core.Context, msg string) error {
	return c.BadRequest(msg)
}

func ErrUnauthorized(c *core.Context) error {
	return c.Unauthorized("unauthorized")
}

func ErrForbidden(c *core.Context) error {
	return c.Forbidden("forbidden")
}

func ErrNotFound(c *core.Context, entity string) error {
	return c.NotFound(entity + " not found")
}

func ErrInternal(c *core.Context) error {
	return c.InternalError("internal server error")
}

func ErrValidation(c *core.Context, ve *core.ValidationError) error {
	return c.Status(core.StatusBadRequest).JSON(map[string]any{
		"error":  "validation failed",
		"fields": ve.Fields,
	})
}
