package main

import (
	"go-crud/controllers"
	"go-crud/repositories"
)

func main() {
	repositories.ConnectToDb()
	controllers.AddRouters()
}
