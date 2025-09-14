## Incremental Tables / Lists

They are used to store large amounts of data of one category read in groups.

For example, chat messages.

By saving them in a table:

127.0.0.1:5844/save_inc/{table_chats}/{key_chat_id}

We can add additional messages.

When a user enters the chat, we can load the last 100 messages and retrieve them with a single command using only 2 IO operations.

---

IncTables also optimize the database's resource usage.

Each classic entry uses a small but always occupied portion of RAM.

An IncTable appears in the database as one entry, but it can contain a very large number of entries.

### IncTables also have their drawbacks
The only way to read the entries is to read them:
1. by knowing their specific ID
2. by reading a specific number starting from a specific location

