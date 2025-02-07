package public_api_v1

import (
	routes "TsunamiDB/servers/public-api/v1/routes"
	"fmt"
	"log"
	"net/http"
)

func RunPublicApi_v1(port int) {
	http.HandleFunc("/save/", routes.Save) // save
	http.HandleFunc("/read/", routes.Read) // read
	http.HandleFunc("/free/", routes.Free) // delete

	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		log.Fatal(err)
	}
}
