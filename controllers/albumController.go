package controllers

import (
	"github.com/gin-gonic/gin"
	"go-crud/middlewares"
	"go-crud/services"
	"log"
)

func AddRouters() {
	router := gin.Default()

	router.GET("/", func(c *gin.Context) {
		c.String(200, "Welcome To This Website")
	})

	api := router.Group("/api")
	{
		public := api.Group("/auth")
		{
			public.POST("/login", services.Login)
			public.POST("/signup", services.Signup)
		}

		protected := api.Group("/albums").Use(middlewares.AuthHandler())
		{
			protected.GET("/", services.GetAlbums)
			protected.GET("/:id", services.GetAlbumByID)
			protected.GET("/artist/:artist", services.GetAlbumByArtist)
			protected.POST("/", services.PostAlbums)
			protected.PUT("/:id", services.UpdateAlbum)
			protected.DELETE("/:id", services.DeleteAlbum)
		}
		weather := api.Group("/weather").Use(middlewares.AuthHandler())
		weather.GET("/:lat/:long", services.CurrentWeather)

	}

	err := router.Run("localhost:8080")

	if err != nil {
		log.Fatal(err)
	}
}
