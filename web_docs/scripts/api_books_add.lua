function api_books_add() 
    local method = request.method
    local path = request.url
    local req_body = request.body
    local headers = request.headers
    local db = request.db
    local json = request.json
    
    -- init logs
    local log = {}
    table.insert(log, "$ start simulation")

    -- init res
    local res_status = 0
    local res_body = [[empty]]

    -- check method
    if method ~= "POST" then
        table.insert(log, "$ Method Not Allowed")
        res_status = 405
        res_body = [[{ "error": "Method Not Allowed" }]]
        return {
            response = {
                status = res_status,
                body = res_body
            },
            log = log,
            db = db
        }
    end

    -- sprawdzenie tokena autoryzacji
    table.insert(log, "$ auth user")
    local token = headers["auth_token"]
    if token ~= "user_token" then
        table.insert(log, "$ brak autoryzacji: token = " .. tostring(token))
        return {
        response = {
            status = 401,
            body = [[{ "message": "Brak autoryzacji" }]]
        },
        log = log
        }
    end

    -- edit db
    table.insert(log, "$ save generated book UUID in inventory_numbers table in db")
    table.insert(log, "$ edit books table in db")
    table.insert(db, {
        title = json.title,
        author = json.author,
        publisher = json.publisher,
        year = json.year,
        isbn = json.isbn,
        pages = json.pages,
        description = json.description,
        tags = json.tags,
        shelf = json.shelf,
        copies = json.copies
    })

    -- normal res
    res_status = 200
    res_body = "{\"message\":\"Książka dodana\"}"

    return {
        response = {
            status = res_status,
            body = res_body
        },
        log = log,
        db = db
    }

end