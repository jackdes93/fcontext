package sctx

import "context"

type Component interface {
	ID() string
	InitFlags()
	Activate(ctx context.Context, service ServiceContext) error
	Stop(ctx context.Context) error
	Order() int
}

func componentOrder(c Component) int {
	type orderer interface{ Order() int }
	if o, ok := any(c).(orderer); ok {
		return o.Order()
	}
	return 100
}
