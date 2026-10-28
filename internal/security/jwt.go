package security

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Role string `json:"role"`
	Name string `json:"name"`
	Ver  int    `json:"ver"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	key  []byte
	days int
}

func NewJWTManager(master []byte, days int) *JWTManager {
	return &JWTManager{key: DeriveKey(master, "jwt"), days: days}
}

func (m *JWTManager) Issue(userID int64, role, name string, ver int) (string, time.Time, error) {
	now := time.Now().UTC()
	exp := now.Add(time.Duration(m.days) * 24 * time.Hour)
	claims := Claims{
		Role: role,
		Name: name,
		Ver:  ver,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(m.key)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

func (m *JWTManager) Parse(token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return m.key, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}
