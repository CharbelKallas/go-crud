package repositories

import (
	"database/sql"
	"errors"
	"fmt"
	"go-crud/models"
)

func CreateUserRecord(user models.User) error {
	_, err := Db.Exec("INSERT INTO user (name, email, password) VALUES (?, ?, ?)", user.Name, user.Email, user.Password)
	if err != nil {
		return fmt.Errorf("CreateUserRecord: %v", err)
	}
	return nil
}

func UserByEmail(email string) (models.User, error) {
	var user models.User

	row := Db.QueryRow("SELECT * FROM user WHERE email = ?", email)
	if err := row.Scan(&user.ID, &user.Name, &user.Email, &user.Password); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user, fmt.Errorf("UserByEmail %s: no such user", email)
		}
		return user, fmt.Errorf("UserByEmail %s: %v", email, err)
	}
	return user, nil
}
