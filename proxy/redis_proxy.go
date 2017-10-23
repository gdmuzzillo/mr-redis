package main
// test
import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
)

//ConfigMap A map of name of the proxy vs its actually backend endpoint
var ConfigMap map[string]Entry

//Config json config structure for the proxy
type Config struct {
	HTTPPort string  //HTTPPort server Port number that we should bind to
	Entries  []Entry //Entries List of proxy entries
}

//Entry Representation of each entry in the proxy config
type Entry struct {
	Name string
	Pair PorxyPair
}

//PorxyPair The actuall proxy pair from (bind port) to actual port
type PorxyPair struct {
	From string //IP:PORT pair
	To   string //IP:PORT pair
}

//HTTPUpdate This structure is used by the HTTP PUT request to change the IP address of the destination on the fly
type HTTPUpdate struct {
	Name string
	Addr string
}

//HandleConnection Actuall proxy implementation per client. Untimatly this performs a implments a duplex io.Copy
func HandleConnection(E Entry) error {

	var CurrentE Entry //A Temp variable to get the latest Desination proxy value
	var OK bool

	log.Printf("HandleConnection() %v", E)
	src, err := net.Listen("tcp", E.Pair.From)
	if err != nil {
		log.Printf("Error binding to the IP %v", err)
		return err
	}
	defer src.Close()

	//Add this in the global Map so that it can be updated dynamically by HTTP apis
	ConfigMap[E.Name] = E

	for {
		conn, err := src.Accept()
		if err != nil {
			log.Printf("Error accepting a new connection %v", err)
			continue
		}

		//Get the latest Entry from the MAP because it migh thave been updated on the fly.
		if CurrentE, OK = ConfigMap[E.Name]; !OK {
			log.Printf("Error Proxy entry is incorrect / empty for %s", E.Name)
			conn.Close()
			continue
		}

		//Start a Lamda for performing the proxy
		//F := From Connection
		//T := To Connection
		//This proxy will simply transfer everything from F to T net.Conn
		go func(E Entry, F net.Conn) {

			T, err := net.Dial("tcp", E.Pair.To)
			if err != nil {
				log.Printf("Unable to connect to the Destination %s %v", E.Pair.To, err)
				return
			}
			defer T.Close()
			defer F.Close()

			go io.Copy(F, T)
			io.Copy(T, F)

		}(CurrentE, conn)
	}
}

//HandleHTTPUpdate Call beack to handle /Update/ HTTP call back
func HandleHTTPUpdate(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hi there, Going to Update %s! Method=%s\n", r.URL.Path[1:], r.Method)
	if r.Method == "PUT" {
		//This can be used for updating an existing variable
		content, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			fmt.Fprintf(w, "Error understanding the Body %v", err)
			log.Printf("Error understanding the Body %v", err)
			return
		}

		var val HTTPUpdate
		var CurrentE Entry
		var OK bool
		err = json.Unmarshal(content, &val)
		if err != nil {
			fmt.Fprintf(w, "Wrong json format %v", err)
			log.Printf("Wrong json format %v", err)
			return
		}
		if CurrentE, OK = ConfigMap[val.Name]; !OK {
			log.Printf("Error Proxy entry is incorrect / empty for %s", val.Name)
			fmt.Fprintf(w, "Error Proxy entry is incorrect / empty for %s", val.Name)
			return
		}
		log.Printf("Updating From porxy for %s From %s TO %s", val.Name, CurrentE.Pair.To, val.Addr)
		CurrentE.Pair.To = val.Addr
		ConfigMap[val.Name] = CurrentE
		return
	}
	return
}

//HandleHTTPGet call back to handle /Get/ HTTP callback
func HandleHTTPGet(w http.ResponseWriter, r *http.Request) {
	retBytes, err := json.MarshalIndent(ConfigMap, " ", "  ")
	if err != nil {
		log.Printf("Error Marshalling HandleHTTPGet() %v", err)
		fmt.Fprintf(w, "Error Marshalling HandleHTTPGet() %v", err)
		return

	}
	fmt.Fprintf(w, "Current Config: %s", string(retBytes))
	return
}

func main() {

	var Cfg Config

	//Initialize the global Config map
	ConfigMap = make(map[string]Entry)

	//Read a config file that has json update the config files
	cfgFileName := flag.String("config", "./config.json", "Supply the location of MrRedis configuration file")
	flag.Parse()

	log.Printf("The config file name is %s ", *cfgFileName)
	cfgFile, err := ioutil.ReadFile(*cfgFileName)

	if err != nil {
		log.Printf("Error Reading the configration file. Resorting to default values")
	}
	err = json.Unmarshal(cfgFile, &Cfg)
	if err != nil {
		log.Fatalf("Error parsing the config file %v", err)
		return
	}
	log.Printf("Configuration file is = %v", Cfg)

	//Hanlde each connection

	for _, E := range Cfg.Entries {
		go HandleConnection(E)
	}

	http.HandleFunc("/Update/", HandleHTTPUpdate)
	http.HandleFunc("/Get/", HandleHTTPGet)
	log.Fatal(http.ListenAndServe(":"+Cfg.HTTPPort, nil))

	//Wait indefinitely
	waitCh := make(chan bool)
	<-waitCh

}
