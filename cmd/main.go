package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	vaultapi "github.com/hashicorp/vault/api"
)

func main() {
	client, err := vaultapi.NewClient(vaultapi.DefaultConfig())
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		secret, err := client.Logical().Read(os.Getenv("SECRET_PATH"))
		if err != nil {
			fmt.Fprint(w, "I can't read that secret :(")
		} else {
			fmt.Fprint(w, secret.Data["hello"])
		}
	})

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
