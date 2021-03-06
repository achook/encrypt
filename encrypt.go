/*
 * Encrypter encypt small files using AES-256
 * Key derivation is done using scrypt algorithm
 */

package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	saltLength = 32
	keyLength  = 32
	errorColor = "\033[91m"
	endColor   = "\033[0m"
)

func main() {
	// Initialize variables for flags
	var password, filePath, outputPath string
	var encryptFlag, decryptFlag bool

	// Bind flags to variables
	flag.StringVar(&password, "p", "", "Password")
	flag.StringVar(&filePath, "f", "", "Path to the file")
	flag.StringVar(&outputPath, "o", "", "Output path")
	flag.BoolVar(&encryptFlag, "e", false, "Encrypt the file")
	flag.BoolVar(&decryptFlag, "d", false, "Decrypt the file")

	// Parse flags
	flag.Parse()

	// Check if flags were used correctly
	if encryptFlag && decryptFlag {
		throwError("You mustn't use '-e' i '-d' flags simultaneously")
	}
	if !encryptFlag && !decryptFlag {
		throwError("You must use '-e' or '-d' flag")
	}
	if password == "" {
		throwError("You must provide a password using '-p' parameter")
	}
	if filePath == "" {
		throwError("You must provide a path to the file using '-f' parameter")
	}

	// Read input file
	inputFile, err := ioutil.ReadFile(filePath)
	if err != nil {
		throwError("Error reading input file: ", err)
	}

	var outputFile []byte

	if encryptFlag {
		// If user's encrypting file

		// Check the extension and its length
		ext := []byte(filepath.Ext(filePath))
		extLen := []byte(string(len(ext) - 1))

		// Just check if extension isn't too long. Nearly impossible because of file systems
		if len(extLen) > 256 {
			throwError("File extensiong musn't be longer than 256")
		}

		// Make file for output by appending extension and its length to file provided by user
		file := append(extLen, ext...)
		file = append(file, inputFile...)

		// And finally decrypt the file
		outputFile, err = encrypt(file, []byte(password))
		if err != nil {
			throwError("Error encrypting file: ", err)
		}

		// Make the final path for encrypted file by exchanging current extension with .enc
		// Extnesion and final file name isn't important. Just some sugar
		if outputPath == "" {
			outputPath = filePath[:len(filePath)-len(filepath.Ext(filePath))]
			outputPath = outputPath + ".enc"
		}
	}

	if decryptFlag {
		// If user's encrypting file

		// Decrypt the file provided by user
		file, err := decrypt(inputFile, []byte(password))
		if err != nil {
			throwError("Error decrypting file: ", err)
		}

		// Extract the extension from plaintexy
		extLen := int(file[0])
		ext := string(file[1 : extLen+2])
		outputFile = file[extLen+2:]

		// If user wants to change the output path check if the extension matches
		// If not, ask what to do
		if outputPath != "" && (filepath.Ext(outputPath) != ext) {
			reader := bufio.NewReader(os.Stdin)

			fmt.Println("Extension you provided doesn't match original file extension (", ext, ")")
			fmt.Print("Continue [y], change extension to correct one [C] or abort [n]? ")

			// Trim CR and LF
			input, _ := reader.ReadString('\n')
			input = strings.Trim(input, "\n")
			input = strings.Trim(input, "\r")

			switch input {
			case "y":
			case "Y":
				break

			case "n":
			case "N":
				os.Exit(0)

			default:
				userExtLen := len(filepath.Ext(outputPath))
				outputPath = outputPath[:len(outputPath)-userExtLen]
				outputPath = outputPath + ext
				outputPath, _ = filepath.Abs(outputPath)
				break
			}
		}

		// If user didn't specify output path just change the extension to proper one
		if outputPath == "" {
			outputPath = filePath[:len(filePath)-len(filepath.Ext(filePath))]
			outputPath = outputPath + ext
		}
	}

	// Write file
	err = ioutil.WriteFile(outputPath, outputFile, 0777)
	if err != nil {
		throwError("Error saving file: ", err)
	}

}

func makeKey(password []byte, salt []byte) ([]byte, error) {
	// Return key generated by scrypt
	// More info about values provided: https://godoc.org/golang.org/x/crypto/scrypt#Key
	return scrypt.Key(password, salt, 16384, 8, 1, keyLength)
}

func encrypt(plaintext []byte, password []byte) ([]byte, error) {

	// Get salt from cypto/rand
	salt := make([]byte, saltLength)
	_, err := io.ReadFull(rand.Reader, salt)
	if err != nil {
		return nil, err
	}

	// Generate key using user password and salt
	// Set keyLength constant according to https://golang.org/pkg/crypto/aes/#NewCipher
	key, err := makeKey(password, salt)
	if err != nil {
		return nil, err
	}

	// Make new AES cipher
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Make block cipher using Galois Counter Mode
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	// Get nonce from cypto/rand
	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, err
	}

	// Encrypt plaintext
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// Append salt used to ciphertext
	ciphertext = append(ciphertext[:], salt[:]...)

	return ciphertext, nil
}

func decrypt(ciphertext []byte, password []byte) ([]byte, error) {
	// Check if file isn't too small
	if saltLength > len(ciphertext) {
		return nil, fmt.Errorf("File is wrong or broken")
	}

	// Extract salt to generate key
	salt := ciphertext[len(ciphertext)-saltLength:]

	// Make key using given password and salt
	key, err := makeKey(password, salt)
	if err != nil {
		return nil, err
	}

	// Make new AES cipher
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Make block cipher using Galois Counter Mode
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	// Check again if ciphertext isn't too short
	nonceSize := gcm.NonceSize()
	if len(ciphertext[:len(ciphertext)-saltLength]) < nonceSize {
		return nil, fmt.Errorf("Ciphertext is too short")
	}

	// Make room for nonce
	nonce := ciphertext[:nonceSize]
	ciphertext = ciphertext[nonceSize : len(ciphertext)-saltLength]

	// Decrypt the file
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func throwError(data ...interface{}) {
	fmt.Print(errorColor)
	fmt.Print("Error: ")
	fmt.Println(data...)
	fmt.Print(endColor)
	os.Exit(1)
}
