package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	jwt "github.com/golang-jwt/jwt/v4"
)

//
// To generate a token from secret:
//
//   token := jwt.New(jwt.SigningMethodHS256)
//   tokenString, _ := token.SignedString(jwtSecret)
//   fmt.Printf(tokenString)
//

func CreateJwtMiddleware(jwtSecret []byte) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			authorizationHeader := req.Header.Get("Authorization")

			if authorizationHeader == "" {
				w.WriteHeader(401)
				_ = json.NewEncoder(w).Encode("An authorization header is required")
				return
			}

			bearerToken := strings.Split(authorizationHeader, " ")

			if len(bearerToken) != 2 {
				w.WriteHeader(401)
				_ = json.NewEncoder(w).Encode("Invalid authorization token")
				return
			}

			token, err := jwt.Parse(bearerToken[1], func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("invalid authorization token")
				}

				return jwtSecret, nil
			})

			if err != nil {
				w.WriteHeader(401)
				_ = json.NewEncoder(w).Encode(err.Error())
				return
			}

			if !token.Valid {
				w.WriteHeader(401)
				_ = json.NewEncoder(w).Encode("Invalid authorization token")
				return
			}

			next(w, req)
		})
	}
}
