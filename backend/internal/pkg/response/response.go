package response

import (
	"github.com/gin-gonic/gin"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
)

type Response struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(200, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(201, Response{
		Code:    0,
		Message: "created",
		Data:    data,
	})
}

func Error(c *gin.Context, err *errcode.AppError) {
	resp := Response{
		Code:      err.Code,
		Message:   err.Message,
		RequestID: c.GetString("request_id"),
	}
	if err.Data != nil {
		resp.Data = err.Data
	}
	c.JSON(err.HTTP, resp)
	c.Abort()
}

func ErrorWithMsg(c *gin.Context, err *errcode.AppError, msg string) {
	c.JSON(err.HTTP, Response{
		Code:      err.Code,
		Message:   msg,
		RequestID: c.GetString("request_id"),
	})
	c.Abort()
}
