package goichi

type Route struct {
	info *RouteInfo
}

type Routes []*Route

func newRoute(info *RouteInfo) *Route {
	return &Route{info: info}
}

func (rs Routes) Summary(s string) Routes {
	for _, r := range rs {
		r.Summary(s)
	}
	return rs
}

func (rs Routes) Tags(tags ...string) Routes {
	for _, r := range rs {
		r.Tags(tags...)
	}
	return rs
}

func (rs Routes) ResponseModel(model any) Routes {
	for _, r := range rs {
		r.ResponseModel(model)
	}
	return rs
}

func (rs Routes) RequestModel(model any) Routes {
	for _, r := range rs {
		r.RequestModel(model)
	}
	return rs
}

func (rs Routes) NoAuth() Routes {
	for _, r := range rs {
		r.NoAuth()
	}
	return rs
}

func (rs Routes) HideDoc() Routes {
	for _, r := range rs {
		r.HideDoc()
	}
	return rs
}

func (r *Route) ContentType(ct string) *Route {
	r.info.ContentType = ct
	return r
}

func (rs Routes) ContentType(ct string) Routes {
	for _, r := range rs {
		r.ContentType(ct)
	}
	return rs
}

func (r *Route) Summary(s string) *Route {
	r.info.Summary = s
	return r
}

func (r *Route) Tags(tags ...string) *Route {
	r.info.Tags = tags
	return r
}

func (r *Route) ResponseModel(model any) *Route {
	r.info.ResponseModel = model
	return r
}

func (r *Route) RequestModel(model any) *Route {
	r.info.RequestModel = model
	return r
}

func (r *Route) NoAuth() *Route {
	r.info.NoAuth = true
	return r
}

func (r *Route) HideDoc() *Route {
	r.info.HideDoc = true
	return r
}
