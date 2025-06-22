function api_books_get()
  local method = request.method
  local path = request.url
  local req_body = request.body
  local headers = request.headers
  local db = request.db

  -- logs part:
  local log = {}
  table.insert(log, "$ start simulation")

  if method ~= "GET" then
    table.insert(log, "$ Method Not Allowed")
    return {
      response = {
        status = 405,
        body = [[{ "error": "Method Not Allowed" }]]
      },
      log = log
    }
  end

  -- sprawdzenie tokena autoryzacji
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

  -- Parsowanie inventoryNumber
  if not req_body or #req_body < 1 then
    table.insert(log, "$ body is empty")
    return {
     response = {
       status = 500,
       body = [[{ "error": "some server error" }]]
     },
     log = log
    }
  end

  local inventoryNumber = nil
  if req_body then
    local _, _, value = string.find(req_body, "inventoryNumber%s*:%s*(%d+)")
    if value then
      inventoryNumber = tonumber(value)
    end
  end

  table.insert(log, "$ received inventoryNumber = " .. (inventoryNumber or "nil"))

  -- tworzenie ciała odpowiedzi jako JSON string
  local response_body = ""
  local response_status = 200
  if not inventoryNumber then
    response_status = 404
    response_body = [[{ "message": "Książka o takim numerze nie istnieje" }]]
  elseif inventoryNumber == 200 or inventoryNumber == 101 then
    response_body = [[{ "available_inventory_number": "]] .. tostring(inventoryNumber) .. [[" }]]
  else
    response_status = 400
    response_body = [[{ "message": "Brak poprawnego numeru inwentarzowego w ścieżce" }]]
  end

  if inventoryNumber == 100 then
    local result = {
      response = {
        status = 200,
        body = [[{
"next_available_date": "2025-05-31",
"error": "Książka zarezerwowana \u2013 oczekuje na odbiór\n !!! Brak gwarancji, że książka będzie dostępna"
}]]
      },
      log = log,
      db = db
    }
    return result
  end

  -- jeżeli inventoryNumber

  -- {"available_inventory_number":"101"}

  -- test db change
  -- for i, row in ipairs(db) do
  --   if tonumber(row.inventoryNumber) == 100 then
  --     table.insert(log, "$ changing status of book 100")
  --     db[i].status = "borrowed"
  --     db[1].inventoryNumber = "200"
  --   end
  -- end


  -- wynik końcowy
  local result = {
    response = {
      status = response_status,
      body = response_body
    },
    log = log,
    db = db
  }

  return result
end

-- RESULT:

-- result musi wyglądać zawsze w taki sposób, kolejność wartości w response jest ważne
-- response[0] = status
-- response[1] = body

--  local result = {
--     response = {
--       status = "ok",
--       body = response_body
--     },
--     log = log
--   }