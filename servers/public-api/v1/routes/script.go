package routes

import "net/http"

/*
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


		actions:
			setParent(<child Key>, <parent Key>)
			setChild(<child Key>, <parent Key>)
			changeKeyValue(???)
				> cos co pozwoli zmienic key


	# logs:
		plik specjalnie endkodowany z mapą przechowujący
		logi kto co zmienił, zpis tylko przy wykonaniu
		logs.bin ->

*/

func Script(w http.ResponseWriter, r *http.Request) {

}
