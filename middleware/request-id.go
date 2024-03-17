package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
)

func RequestId() func(c *gin.Context) {
	return func(c *gin.Context) {
		type contextKey string
		const requestIdKey contextKey = "X-Oneapi-Request-Id"

		id := helper.GenRequestID()
		c.Set(logger.RequestIdKey, id)
		ctx := context.WithValue(c.Request.Context(), requestIdKey, id)
		c.Request = c.Request.WithContext(ctx)
		c.Header(logger.RequestIdKey, id)
		c.Next()
	}
}
