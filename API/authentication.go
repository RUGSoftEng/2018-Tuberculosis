package main

import (
	"database/sql"
	"encoding/json"
	"github.com/dgrijalva/jwt-go"
	_ "github.com/go-sql-driver/mysql" // anonymous import
	"github.com/gorilla/mux"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	http "net/http"
	"strconv"
)

//HashPassword hashes the given string
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

//This function validates a password against a specific user, and issues a JWT Token
func login(r *http.Request, ar *APIResponse) {
	cred := UserValidation{}
	err := json.NewDecoder(r.Body).Decode(&cred)
	if err != nil {
		ar.setErrorAndStatus(http.StatusInternalServerError, err, "Failed to decode user credentials")
		return
	}
	var password string
	err = db.QueryRow(`SELECT pass_hash FROM Accounts WHERE username=?`, cred.Username).Scan(&password)
	if err != nil {
		ar.setErrorAndStatus(http.StatusInternalServerError, err, "Database failure")
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(password), []byte(cred.Password))
	if err != nil {
		ar.setErrorAndStatus(http.StatusUnauthorized, err, "Unauthorized")
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": cred.Username,
		"password": cred.Password})
	tokenString, err := token.SignedString([]byte("secret"))

	if err != nil {
		ar.setErrorAndStatus(http.StatusInternalServerError, err, "Failed to generate JWT token")
		return
	}
	var tokenID int
	err = db.QueryRow(`SELECT id FROM Accounts WHERE username=?`, cred.Username).Scan(&tokenID)
	ar.Data = JWToken{Token: tokenString, ID: tokenID}
}

func parseToken(in JWToken, ar *APIResponse) {
	content := in.Token
	id := in.ID
	token, err := jwt.Parse(content, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("There was an error")
		}
		return []byte("secret"), nil
	})
	if err != nil {
		ar.setErrorAndStatus(http.StatusUnauthorized, err, "Unauthorized")
		return
	}
	if claims, ok := token.Claims.(jwt.MapClaims); !ok || !token.Valid {
		ar.setErrorAndStatus(http.StatusUnauthorized, err, "Unauthorized")

		var user UserValidation
		err = mapstructure.Decode(claims, &user)
		if err != nil {
			ar.setErrorAndStatus(http.StatusInternalServerError, err, "Database failure")
			return
		}
		var pwd string
		err := db.QueryRow(`SELECT pass_hash FROM Accounts WHERE username=?`, user.Username).Scan(&pwd)
		if err != nil {
			ar.setErrorAndStatus(http.StatusInternalServerError, err, "Database failure")
			return
		}
		var readID int
		err = db.QueryRow(`SELECT id FROM Accounts WHERE username=?`, user.Username).Scan(&readID)
		if err == sql.ErrNoRows {
			ar.setErrorAndStatus(http.StatusUnauthorized, err, "Unauthorized")
			return
		}
		if err != nil {
			ar.setErrorAndStatus(http.StatusInternalServerError, err, "Database failure")
			return
		}
		// DO NOT REMOVE THE FOLLOWING LINES
		if id != readID {
			ar.setErrorAndStatus(http.StatusUnauthorized, err, "Unauthorized")
			return
		}
		// UP UNTIL HERE
		err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(pwd))
		if err != nil {
			ar.setErrorAndStatus(http.StatusUnauthorized, err, "Unauthorized")
			return
		}
		return
	}
	/*
		ar.StatusCode = http.StatusBadRequest
		ar.Error = errors.Wrap(err, "Authentication failed: Mismatching credentials")
	*/
}

// Token authentication will probably be embedded in all the request that are give access
// to restricted contents, this functions is only for test purposes, but it uses the
// tokenParse() function that will do the core of the work

// I think this function shield potentially return an error if the credentials are invalid, instead of the boolean
func authenticate(r *http.Request, ar *APIResponse) {
	vars := mux.Vars(r)
	id1 := vars["id"]
	id, err := strconv.Atoi(id1)
	if err != nil {
		ar.setErrorAndStatus(http.StatusInternalServerError, err, "Conversion failed")
		return
	}
	token := r.Header.Get("access_token")
	pass := JWToken{Token: token, ID: id}
	parseToken(pass, ar)
}

func authWrapper(handler func(r *http.Request, ar *APIResponse)) func(*http.Request, *APIResponse) {
	return func(r *http.Request, ar *APIResponse) {

		authenticate(r, ar)
		if ar.Error != nil {
			return
		}
		handler(r, ar)
	}
}
