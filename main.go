// Package main generates web project.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

func recursiveSearchReplaceFiles(fullpath string, replacers map[string]string) error {
	fileOrDirList := []string{}
	err := filepath.Walk(fullpath, func(path string, f os.FileInfo, err error) error {
		fileOrDirList = append(fileOrDirList, path)
		return nil
	})

	if err != nil {
		return err
	}

	for _, fileOrDir := range fileOrDirList {
		fileInfo, _ := os.Stat(fileOrDir)
		if !fileInfo.IsDir() {
			for oldString, newString := range replacers {
				log.Print("Replacing " + oldString + " -> '" + newString + "' on " + fileOrDir)

				contentBytes, _ := ioutil.ReadFile(fileOrDir)
				newContentBytes := bytes.Replace(contentBytes, []byte(oldString), []byte(newString), -1)

				err := ioutil.WriteFile(fileOrDir, newContentBytes, fileInfo.Mode())
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func createDatabase(name string) {
	log.Print("Creating a database named " + name + "...")
	if exec.Command("dropdb", name).Run() == nil {
		log.Print("Dropped existing PostgreSQL database: " + name)
	}

	if exec.Command("createdb", name).Run() != nil {
		log.Print("Unable to create PostgreSQL database: " + name)
	}
}

func main() {
	dir := flag.String("dir", "", "directory of project relative to $GOPATH/src/")
	flag.Parse()

	if *dir == "" {
		log.Fatal("dir option is missing.")
	}

	dirChunks := strings.Split(*dir, "/")
	repoName := dirChunks[len(dirChunks)-1]
	repoUser := dirChunks[len(dirChunks)-2]

	fullpath := os.ExpandEnv(filepath.Join("$GOPATH", "src", *dir))

	// 1. Create target directory
	log.Print("Creating " + fullpath + "...")
	err := os.MkdirAll(fullpath, 0755)
	if err != nil {
		log.Fatal(err)
	}

	// 2. Copy everything under blank directory to target directory.
	log.Print("Copying a blank project to " + fullpath + "...")
	if output, err := exec.Command("cp", "-rf", "./blank/", fullpath).CombinedOutput(); err != nil {
		log.Fatal(string(output))
	}

	// 3. Interpolate placeholder variables on the new project.
	log.Print("Replacing placeholder variables to " + repoUser + "/" + repoName + "...")
	replacers := make(map[string]string)
	replacers["$GO_BOOTSTRAP_REPO_USER"] = repoUser
	replacers["$GO_BOOTSTRAP_REPO_NAME"] = repoName
	if err := recursiveSearchReplaceFiles(fullpath, replacers); err != nil {
		log.Fatal(err)
	}

	// 4. Create PostgreSQL databases.
	createDatabase(repoName)
	createDatabase(repoName + "-test")

	// 5.a. go get github.com/mattes/migrate.
	log.Print("Installing github.com/mattes/migrate...")
	if output, err := exec.Command("go", "get", "github.com/mattes/migrate").CombinedOutput(); err != nil {
		log.Fatal(string(output))
	}

	// 5.b. Run migrations on localhost:5432.
	u, _ := user.Current()
	for _, name := range []string{repoName, repoName + "-test"} {
		pgDSN := fmt.Sprintf("postgres://%v@localhost:5432/%v?sslmode=disable", u.Username, name)

		log.Print("Running database migrations on " + pgDSN + "...")
		if output, err := exec.Command("migrate", "-url", pgDSN, "-path", filepath.Join(fullpath, "migrations"), "up").CombinedOutput(); err != nil {
			log.Fatal(string(output))
		}
	}

	// 6. Get all application dependencies.
	log.Print("Running go get ./...")
	cmd := exec.Command("go", "get", "./...")
	cmd.Dir = fullpath
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Fatal(string(output))
	}

	// 7. Run tests on newly generated app.
	log.Print("Running go test ./...")
	cmd = exec.Command("go", "test", "./...")
	cmd.Dir = fullpath
	output, _ := cmd.CombinedOutput()
	log.Print(string(output))
}
