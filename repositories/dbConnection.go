package repositories

import (
	"database/sql"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"log"
	"os"
)

var Db *sql.DB

func ConnectToDb() {
	cfg := mysql.Config{
		User:   os.Getenv("DB_USERNAME"),
		Passwd: os.Getenv("DB_PASSWORD"),
		Net:    "tcp",
		Addr:   os.Getenv("DATABASE_ADDR"),
		DBName: os.Getenv("DB_DATABASE"),
	}

	Db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		log.Fatal(err)
	}

	pingErr := Db.Ping()
	if pingErr != nil {
		log.Fatal(pingErr)
	}

	fmt.Println("Connected!")
}
