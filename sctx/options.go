package sctx

type Option func(*serviceCtx)

func WithName(name string) Option {
	return func(s *serviceCtx) { s.name = name }
}

func WithComponent(c Component) Option {
	return func(s *serviceCtx) {
		if _, ok := s.store[c.ID()]; ok {
			return
		}
		s.components = append(s.components, c)
		s.store[c.ID()] = c
	}
}

func WithLogger(l Logger) Option {
	return func(s *serviceCtx) { s.logger = l }
}
