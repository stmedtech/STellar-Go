package bootstrap

import (
	"bufio"
	"math/rand/v2"
	"os"
	"path/filepath"

	golog "github.com/ipfs/go-log/v2"
)

var logger = golog.Logger("stellar-p2p-bootstrap")

var BOOTSTRAPPERS = []string{
	"/ip4/114.32.226.175/tcp/43210/p2p/12D3KooWJ3VruqtQC4g7wvfy7NPqtdJmrWotzki4b2J7D9tYzY9a",
}

func getBootstrappers() ([]string, error) {
	bootstrappers := BOOTSTRAPPERS

	pwd, err := os.Getwd()
	if err != nil {
		return bootstrappers, err
	}
	file, err := os.Open(filepath.Join(pwd, "bootstrappers.txt"))
	if err != nil {
		return bootstrappers, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		bootstrappers = append(bootstrappers, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return bootstrappers, err
	}

	return bootstrappers, nil
}

func init() {
	bootstrappers, readErr := getBootstrappers()
	if readErr != nil {
		logger.Warn(readErr)
		return
	}
	BOOTSTRAPPERS = append(BOOTSTRAPPERS, bootstrappers...)
	for i := range BOOTSTRAPPERS {
		j := rand.IntN(i + 1)
		BOOTSTRAPPERS[i], BOOTSTRAPPERS[j] = BOOTSTRAPPERS[j], BOOTSTRAPPERS[i]
	}
}
