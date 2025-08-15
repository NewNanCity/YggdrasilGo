package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// JWT密钥（从配置中设置）
var jwtSecret []byte

// SetJWTSecret 设置JWT密钥
func SetJWTSecret(secret string) {
	jwtSecret = []byte(secret)
}

// JWTClaims JWT声明
type JWTClaims struct {
	UserID    string `json:"sub"`  // 用户ID
	ProfileID string `json:"spr"`  // 选中的角色ID（可选）
	TokenID   string `json:"yggt"` // 令牌ID
	jwt.RegisteredClaims
}

// GenerateJWT 生成JWT令牌
func GenerateJWT(userID, profileID string, expiration time.Duration) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		UserID:    userID,
		ProfileID: profileID,
		TokenID:   GenerateRandomUUID(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "Yggdrasil-Auth",
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ValidateJWT 验证JWT令牌
func ValidateJWT(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// HashPassword 哈希密码
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword 验证密码
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateRSAKeyPair 生成RSA密钥对
func GenerateRSAKeyPair() (string, string, error) {
	// 生成4096位RSA私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", err
	}

	// 编码私钥
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", "", err
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// 编码公钥
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return string(privateKeyPEM), string(publicKeyPEM), nil
}

// LoadOrGenerateKeyPair 从文件加载或生成密钥对
func LoadOrGenerateKeyPair(privateKeyPath, publicKeyPath string) (string, string, error) {
	// 检查密钥文件是否存在
	privateKeyExists := fileExists(privateKeyPath)
	publicKeyExists := fileExists(publicKeyPath)

	if privateKeyExists && publicKeyExists {
		// 加载现有密钥
		privateKey, err := loadKeyFromFile(privateKeyPath)
		if err != nil {
			return "", "", fmt.Errorf("failed to load private key: %w", err)
		}

		publicKey, err := loadKeyFromFile(publicKeyPath)
		if err != nil {
			return "", "", fmt.Errorf("failed to load public key: %w", err)
		}

		return privateKey, publicKey, nil
	}

	// 生成新的密钥对
	privateKey, publicKey, err := GenerateRSAKeyPair()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate key pair: %w", err)
	}

	// 保存密钥到文件
	if err := saveKeyToFile(privateKeyPath, privateKey); err != nil {
		return "", "", fmt.Errorf("failed to save private key: %w", err)
	}

	if err := saveKeyToFile(publicKeyPath, publicKey); err != nil {
		return "", "", fmt.Errorf("failed to save public key: %w", err)
	}

	return privateKey, publicKey, nil
}

// fileExists 检查文件是否存在
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

// loadKeyFromFile 从文件加载密钥
func loadKeyFromFile(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// saveKeyToFile 保存密钥到文件
func saveKeyToFile(filename, key string) error {
	// 创建目录（如果不存在）
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 保存密钥文件，设置适当的权限
	return os.WriteFile(filename, []byte(key), 0600)
}

// CalculateHash 计算数据的SHA256哈希
func CalculateHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// IsValidUUID 验证UUID格式
func IsValidUUID(uuid string) bool {
	// UUID格式：xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	return uuidRegex.MatchString(uuid)
}
