package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/jackdes93/fcontext"
)

type Middleware = gin.HandlerFunc

type MiddlewareFactory func(service fcontext.ServiceContext) Middleware
