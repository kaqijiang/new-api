package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

// SetS2Router sets up the Semantic Scholar API passthrough routes
func SetS2Router(router *gin.Engine) {
	s2Router := router.Group("/s2")
	s2Router.Use(middleware.CORS())
	s2Router.Use(middleware.TokenAuth())
	{
		// Academic Graph API: /s2/graph/v1/*
		s2Router.Any("/graph/*path", func(c *gin.Context) {
			c.Set("api", "graph")
			controller.SemanticScholarProxy(c)
		})

		// Recommendations API: /s2/recommendations/v1/*
		s2Router.Any("/recommendations/*path", func(c *gin.Context) {
			c.Set("api", "recommendations")
			controller.SemanticScholarProxy(c)
		})

		// Datasets API: /s2/datasets/v1/*
		s2Router.Any("/datasets/*path", func(c *gin.Context) {
			c.Set("api", "datasets")
			controller.SemanticScholarProxy(c)
		})
	}
}
