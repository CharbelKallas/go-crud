package repositories

import (
	"database/sql"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"log"
)

var Db *sql.DB

func ConnectToDb() {
	config, err := godotenv.Read()
	if err != nil {
		log.Fatal("Error reading .env file")
	}

	cfg := mysql.Config{
		User:   config["DB_USERNAME"],
		Passwd: config["DB_PASSWORD"],
		Net:    "tcp",
		Addr:   config["DATABASE_ADDR"],
		DBName: config["DB_DATABASE"],
	}

	Db, err = sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		log.Fatal(err)
	}

	pingErr := Db.Ping()
	if pingErr != nil {
		log.Fatal(pingErr)
	}

	fmt.Println("Connected!")
}
