# Golang CRUD

## About

This code is written with very little experience with Golang.

## Prerequisites

- go 1.21.4,
- go-sql-driver/mysql database access,
- dgrijalva/jwt-go for jwt
- gin-gonic/gin to expose Rest APIs
- joho/godotenv to use set envirement variables inside .env

### API

- POST http://localhost:8080/api/auth/signup/ { "name": "charbelkallas", "email": "ckallas@gmail.com", "password": "
  charbelkallas" }
- POST http://localhost:8080/api/auth/login/ { "email": "ckallas@gmail.com", "password": "charbelkallas" }
- POST http://localhost:8080/api/albums/ { "title": "test", "artist": "test", "price": 1 }
- PUT http://localhost:8080/api/albums/6 { "title": "test12", "artist": "test12", "price": 1 }
- GET http://localhost:8080/api/albums
- GET http://localhost:8080/api/albums/6
- GET http://localhost:8080/api/albums/artist/John
- DELETE http://localhost:8080/api/albums/6