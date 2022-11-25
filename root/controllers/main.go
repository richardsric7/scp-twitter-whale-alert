package root

import (
	root "bantu-monitor/root/services"

	"github.com/gin-gonic/gin"
)

func Init(router *gin.Engine) {

	//Returns organisation running this bantupay api instance
	router.GET("/", func(c *gin.Context) {

		rootInfo := root.GetRootInfo()

		c.JSON(200, rootInfo)
	})

}
