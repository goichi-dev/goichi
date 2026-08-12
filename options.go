package goichi

type RouteOption func(*RouteInfo)

func WithResponse(model any) RouteOption {
	return func(r *RouteInfo) {
		r.ResponseModel = model
	}
}

func WithRequest(model any) RouteOption {
	return func(r *RouteInfo) {
		r.RequestModel = model
	}
}

func WithTags(tags ...string) RouteOption {
	return func(r *RouteInfo) {
		r.Tags = tags
	}
}

func WithSummary(s string) RouteOption {
	return func(r *RouteInfo) {
		r.Summary = s
	}
}
