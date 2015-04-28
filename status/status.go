package status

import "net/http"
import "fmt"
import "github.com/golang/glog"

func statusHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK")
}

func ServeStatus() {
	glog.Infof("Listening on port 8080")
	http.HandleFunc("/status", statusHandler)
	http.ListenAndServe(":8080", nil)
}
