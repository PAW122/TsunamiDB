package fileSystem_v1

/**
@param key -> key to save
@param data -> pointer to encoded data
*/
func SaveElement(key string, data *string) {

}

/**
@param key -> key to read data
@return bool -> true if data was read successfully, false otherwise
@return string -> read data
*/
func ReadElement(key string) (bool, string) {
	return false, ""
}
