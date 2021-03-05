package main

import (
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/",response)
	log.Fatal(http.ListenAndServe(":8080",nil))
}

func response(writer http.ResponseWriter, request *http.Request) {
	outMsg := []byte("This is the root of the web site.  Nothing else to see here, move on")
	_, err := writer.Write(outMsg)
	if err != nil {
		log.Fatal(err)
	}
}