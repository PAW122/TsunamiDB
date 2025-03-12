package config

type Config struct {
	Tsu_network_config Tsu_network_config `json:"tsu_network_config"`
}

type Tsu_network_config struct {
	Servers []Server `json:"servers"`
}

/*
Ip - server ip
type - server type eg:

	backup - bacup all data
	read - only read
	node - read & free sync
	sync-node - multi node db
*/
type Server struct {
	Ip   string `json:"ip"`
	Type string `json:"type"`
}

func LoadConfig(dir string) {

}

/*
server types explained:

backup:
	copy all saved data to secound server only for emergency manual
	backup process. Dont allow automatic read of any data

	* option to move free'd pointers to "deleted" file instead
	of removing for possible future data recovery

mirron:
	copy all actions send from other servers

read:
	allow to try to read data from other server in network if data
	isn't on server witch to read req was send.

	dont allow to save any data to server, only way to update data on
	"read" server is when user is directly sending req to specific server

node:
	allow to read data from server & free outdated information
	if ovverwriten on other server

sync-node:
	allow to read & write & free data from server.
	saved data is +/- eaven split beetwen all servers

	* requires mutch cominucation between servers
	recomended high band & speed connection between servers

*/

/*
notki:

narazie skupić się na "sync-node".
- zarządzanie obciążeniem serwera poprzez przekierowywanie zapisów do inych serwerów



*/

/*
	Recomendations

	when using 2 or more servers with type "node"
	it is recomended to create local LAN high speed network
	for comunication between node's if possible
*/
