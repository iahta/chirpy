package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "chirpy",
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
		Subject:   userID.String(),
	})
	jwtToken, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", fmt.Errorf("error creating token")
	}
	return jwtToken, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	var claims jwt.RegisteredClaims

	token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (interface{}, error) {
		//Ensure the token uses the expected signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tokenSecret), nil
	})
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("error parsing token: %w", err)
	}
	if !token.Valid {
		return uuid.UUID{}, fmt.Errorf("invalid token")
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("error parsing user ID: %w", err)
	}
	return userID, nil

}
