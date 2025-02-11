# script (lua)

+ Script req:
    /script
    body: {
        "on_triger": "onDeleteCascade / onTimeTrigger"
        "triger_key": "test_key"
        "execute": "
            onDeleteCascade(triger_key string, &body_data []bytes) {
                some code examples:
                overwrite_data(new_body_data)
                overwrite_key(new_key)
                
                // uKey - inny / nie związany klucz
                overwrite_uKey(old_key, new_key)
                overwrite_uData(key, new_body_data)
            }    
        "
    }

https://chatgpt.com/share/67a8ee92-c80c-8013-aaa6-89c58b01bba9