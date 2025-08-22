package routes

import "net/http"

/*
	edit:
	można dodać flage do mapy "script"
	np: "script": ptr
	jeżeli flaga się znajduje w map
	to oznacza, że jest skrypt,
	odczytać dane z ptr i wykonać.

	-dane musiały by być zapisywane w spcjalnej tabeli systemowej np system.scripts
	-free() usuwało by dane scriptu, ale save(key) (overwrite) już nie

	*musiało by to być w czymś ala sql?
	np:
	OnDelete <delete(key) | save(key, data) | change_key_to(string) | ErraseData(key)>
	OnReadEncrypted <ErraseData(key) | delete(key)> //oznacza że da się odczytać tylko 1 raz
	OnRead (any read)
	OnOverwrite <save(key, data)>
	CanDisableScriptOverHttp(bool)

	*zamiast podawać parametr key / data / string
	to można użyć getValueFromKey(key)
	i w ten sposób odczytać dane z klucza który
	ma np:
	[
	CanDisableScriptOverHttp(false)
	OnRead[ErraseData(self.key)]
	]
	
	można tego użyć też np do tworzenie kodów promocyjnych i tym podobnych.
	klucze mogą się same usuwać z bazy danych po wykorzystaniu / odczytanie / redeme

	*musi być np flaga --script=enable/disable
	albo opcja podania w header:
	header{
		script_exec: false
	}
	

	przekładało by się to na coś takiego:
	normal_table key_test_map: {... script: {ptr}}
	
	system.script : ptr =
	{
	Uint16 oznaczający akcje np OnDelete ....
	Uint8 oznaczający długość pointerów danych (9, 16, 32, 64)
	Uint8 oznaczający długość pointera key (9, 16, 32, 64)
	Uint<x> oznaczający długość danych data
	Uint<y> oznaczający długość danych key
	[dane: data] gdzie len=x
	[dane: key] gdzie llen=y
	}
	w ten sposób mając 1 pointer możęóćżąć wszystkie potencjalnie potrzebne dane brz problemu i bez dedykowanej mapy.


	w zasadzie to tabela bez mapy bo pointery były by w głównej mapie

	add script to object
	# object scripts
	> przypisywane do objektu
		examples:
		onEditExecute: [<action>]

		"onDeleteCascade": ["deleteKey('relatedKey')"]
		"LogOn: [delete/edit, <key>]"
		"onAccessLog": ["logAccess('userID')"]
		"onTimeTrigger": ["executeAt('2025-02-10T12:00:00Z', 'someAction')"]
		"onChildCountLimit": ["maxChildren('parentKey', 5)"]
*/

func Script(w http.ResponseWriter, r *http.Request) {

}
