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

	można dodać "specjalne-wartości" do używania np:
	getRandomUuid()
	getDate()

	kolejnność działania:
	najpier wykonaj oryginalne polecenie np read/save
	potem wykonaj skrypt

	!zasada
	skrypt nie może dodać wpisu który posiada inny skrypt
	
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

	onTimeTrigger
		implementacja:
		schedule_list - lista która będzie posiadała wszystkie TimeTrigery w postaci:
		[time] [execute(pointerToScript)]

		lista będzie posiadała na końcu najszybsze do wyoknania i na początku najbardziej oddalone.
		minimalny czas wykonania to 1min w przyszłość.

		do ramu będą zapisywane wszystkie wpisy który odbędą się w przeciągu najbliższych 5min.
		co x min worker będzie przechodził po liście aż natrafi na wpis który jest za więcej niż 5min.

		w ten sposó powinno być to dość wydajne

		*robić oddzielne pliki dla oddzielnych dat
		np /planer/<data>
		tak aby w 1 pliku nie było za dużo wpisów
*/

func Script(w http.ResponseWriter, r *http.Request) {

}
