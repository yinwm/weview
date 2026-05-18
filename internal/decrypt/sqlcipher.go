package decrypt

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"wxview/internal/sqlitedb"
)

const (
	PageSize     = 4096
	KeySize      = 32
	SaltSize     = 16
	IVSize       = 16
	HMACSize     = 64
	ReserveSize  = IVSize + HMACSize
	sqliteHeader = "SQLite format 3\x00"
)

var (
	ErrIncorrectKey    = errors.New("incorrect sqlcipher key")
	ErrAlreadyPlainDB  = errors.New("database is already plaintext sqlite")
	ErrInvalidDatabase = errors.New("invalid database")
)

// ValidateRawKey verifies a WeChat 4.x SQLCipher raw page key against page 1.
func ValidateRawKey(page1 []byte, key []byte) bool {
	if len(page1) < PageSize || len(key) != KeySize {
		return false
	}
	if bytes.Equal(page1[:len(sqliteHeader)], []byte(sqliteHeader)) {
		return false
	}
	salt := page1[:SaltSize]
	macKey := deriveRawMACKey(key, salt)
	return validatePageHMAC(page1, 1, macKey)
}

func ValidateRawHexKey(page1 []byte, hexKey string) bool {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return false
	}
	return ValidateRawKey(page1, key)
}

func ReadPage1(path string) ([]byte, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	page1 := make([]byte, PageSize)
	if _, err := io.ReadFull(f, page1); err != nil {
		return nil, nil, err
	}
	if bytes.Equal(page1[:len(sqliteHeader)], []byte(sqliteHeader)) {
		return nil, nil, ErrAlreadyPlainDB
	}
	salt := append([]byte(nil), page1[:SaltSize]...)
	return page1, salt, nil
}

func DecryptFile(ctx context.Context, src string, dst string, hexKey string) error {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return fmt.Errorf("decode key: %w", err)
	}
	if len(key) != KeySize {
		return fmt.Errorf("decode key: expected %d bytes, got %d", KeySize, len(key))
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	if info.Size() < PageSize {
		return fmt.Errorf("%w: file is smaller than one page", ErrInvalidDatabase)
	}

	pageCount := info.Size() / PageSize
	if info.Size()%PageSize != 0 {
		pageCount++
	}

	page1 := make([]byte, PageSize)
	if _, err := io.ReadFull(in, page1); err != nil {
		return err
	}
	if bytes.Equal(page1[:len(sqliteHeader)], []byte(sqliteHeader)) {
		return ErrAlreadyPlainDB
	}
	if !ValidateRawKey(page1, key) {
		return ErrIncorrectKey
	}

	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	macKey := deriveRawMACKey(key, page1[:SaltSize])
	buf := make([]byte, PageSize)
	for pageNum := int64(1); pageNum <= pageCount; pageNum++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := io.ReadFull(in, buf)
		if err != nil {
			if (errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)) && n > 0 {
				clear(buf[n:])
			} else {
				return err
			}
		}

		if isAllZero(buf) {
			if _, err := out.Write(buf); err != nil {
				return err
			}
			continue
		}

		plain, err := decryptPage(buf, int(pageNum), key, macKey)
		if err != nil {
			return err
		}
		if _, err := out.Write(plain); err != nil {
			return err
		}
	}
	return out.Close()
}

// DecryptToCache writes a temporary decrypted database, validates it in-process,
// then atomically replaces dst. A failed decrypt never overwrites the last cache.
func DecryptToCache(ctx context.Context, src string, dst string, hexKey string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := DecryptFile(ctx, src, tmpPath, hexKey); err != nil {
		return err
	}
	if err := ValidateSQLite(ctx, tmpPath); err != nil {
		return fmt.Errorf("validate decrypted sqlite: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, dst)
}

func ValidateSQLite(ctx context.Context, dbPath string) error {
	db, err := sqlitedb.OpenReadOnly(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	var schemaVersion int
	return db.QueryRowContext(ctx, "PRAGMA schema_version;").Scan(&schemaVersion)
}

func SaltHex(page1 []byte) string {
	if len(page1) < SaltSize {
		return ""
	}
	return hex.EncodeToString(page1[:SaltSize])
}

func deriveRawMACKey(encKey []byte, salt []byte) []byte {
	macSalt := make([]byte, len(salt))
	for i := range salt {
		macSalt[i] = salt[i] ^ 0x3a
	}
	return pbkdf2HMACSHA512(encKey, macSalt, 2, KeySize)
}

func validatePageHMAC(page []byte, pageNum int, macKey []byte) bool {
	if len(page) < PageSize {
		return false
	}
	dataEnd := PageSize - ReserveSize + IVSize
	stored := page[dataEnd : dataEnd+HMACSize]
	mac := hmac.New(sha512.New, macKey)
	offset := 0
	if pageNum == 1 {
		offset = SaltSize
	}
	mac.Write(page[offset:dataEnd])
	var pg [4]byte
	binary.LittleEndian.PutUint32(pg[:], uint32(pageNum))
	mac.Write(pg[:])
	return hmac.Equal(mac.Sum(nil), stored)
}

func decryptPage(page []byte, pageNum int, encKey []byte, macKey []byte) ([]byte, error) {
	if !validatePageHMAC(page, pageNum, macKey) {
		return nil, fmt.Errorf("page %d hmac verification failed", pageNum)
	}
	offset := 0
	if pageNum == 1 {
		offset = SaltSize
	}

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	encrypted := append([]byte(nil), page[offset:PageSize-ReserveSize]...)
	iv := page[PageSize-ReserveSize : PageSize-ReserveSize+IVSize]
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(encrypted, encrypted)

	if pageNum == 1 {
		out := make([]byte, 0, PageSize)
		out = append(out, []byte(sqliteHeader)...)
		out = append(out, encrypted...)
		out = append(out, make([]byte, ReserveSize)...)
		return out, nil
	}

	out := make([]byte, 0, PageSize)
	out = append(out, encrypted...)
	out = append(out, make([]byte, ReserveSize)...)
	return out, nil
}

func isAllZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func pbkdf2HMACSHA512(password []byte, salt []byte, iter int, keyLen int) []byte {
	if iter <= 0 || keyLen <= 0 {
		return nil
	}
	hLen := sha512.Size
	numBlocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, numBlocks*hLen)
	var blockIndex [4]byte

	for block := 1; block <= numBlocks; block++ {
		binary.BigEndian.PutUint32(blockIndex[:], uint32(block))
		mac := hmac.New(sha512.New, password)
		mac.Write(salt)
		mac.Write(blockIndex[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)

		for i := 1; i < iter; i++ {
			mac = hmac.New(sha512.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}
