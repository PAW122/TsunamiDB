# internam TsunamiDB functions list:

+ encode string data, add metedata required for db
    > @param string
    ```
    encoder_v1.Encode("Hello, World")
    ```

+ decode readed data from binary
    > @param []byte
    ```
    encoder_v1.Decode(data)
    ```

+ save data to file 
    > @param Encode() res
    > @param string file name
    ```
    dataManager_v1.SaveDataToFile(encoded, "data.bin")
    ```

+ save data to map with key id value
    > @param string key
    > @param string file name
    > @param dataStart pointer
    > @param dataEnd pointer
    ```
    fileSystem_v1.SaveElementByKey("test5", "data.bin", int(startPtr), int(endPtr))
    ```

+ read data from file
    > @param string file name
    > @param dataStart pointer
    > @param dataEnd pointer
    ```
    dataManager_v1.ReadDataFromFile("data.bin", int64(fs_data.StartPtr), int64(fs_data.EndPtr))
    ```

+ get metadata by key id
    > @param string key id
    ```
    fileSystem_v1.GetElementByKey("test6")
    ```

+ delete key id from fileSystem map
    > * after use data is still readable if u have pointer but u can not get pointers to data using GetElementByKey()
    > @param string key id 
    ```
    fileSystem_v1.RemoveElementByKey("test6")
    ```

+ free memory - defragmentation of db bin file
    > MarkAsFree marks space used by data assigned to specific key and allows to ovverwrite it by next incomng data with size = | < then freed data
    > @param string key id
    > @param string file name
    > @param dataStart pointer
    > @param dataEnd pointer 
    ```
    defragmentationManager.MarkAsFree("test6", "data.bin", int64(fs_data.StartPtr), int64(fs_data.EndPtr))
    ```