window.addEventListener("load", () => {
fetch(`/api/docs?_=${Date.now()}`)
  .then((res) => res.json())
  .then((docs) => {
    const sidebar = document.getElementById("sidebar");
    const details = document.getElementById("details");

    // Grupowanie endpointów po kategorii
    const grouped = {};
    docs.forEach((doc) => {
      if (!grouped[doc.category]) grouped[doc.category] = [];
      grouped[doc.category].push(doc);
    });

    // Dla każdej kategorii stwórz kontener z możliwością rozwijania
    Object.entries(grouped).forEach(([category, endpoints]) => {
      const section = document.createElement("div");
      const header = document.createElement("button");
      header.textContent = category.toUpperCase();
      header.className = "category-btn";

      const list = document.createElement("div");
      list.style.display = "none";

      header.onclick = () => {
        list.style.display = list.style.display === "none" ? "block" : "none";
      };

      endpoints.forEach((doc) => {
        const btn = document.createElement("button");
        btn.className = `endpoint-btn`;

        const hasLuaFunc = !!doc.luaFunc;
        const hasTestTable = !!doc.defaultDB;
        const hadMarkdown = !!doc.markdown;

        btn.innerHTML = `
              <span class="method-badge method-${doc.method.toLowerCase()}">${
          doc.method
        }</span>
              <span class="endpoint-text">${doc.endpoint}</span>
              ${hasLuaFunc ? `<span class="test-badge">test</span>` : ""}
              ${hasTestTable ? `<span class="table-badge">DB</span>` : ""}
              ${hadMarkdown ? `<span class="markdown-badge">MD</span>` : ""}
          `;

        // markdowns
        const container = document.createElement("div");
        container.className = "endpoint-container";

        // dodaj główny przycisk
        container.appendChild(btn);

        // dodaj markdown-y dopiero POD endpointem
        if (doc.markdown && Array.isArray(doc.markdown)) {
          const markdownWrapper = document.createElement("div");
          markdownWrapper.className = "markdown-wrapper"; // do stylowania jako małe

          doc.markdown.forEach((path) => {
            const mdBtn = document.createElement("button");
            mdBtn.textContent = path.split("/").pop(); // np. api_books_add.md
            mdBtn.className = "markdown-btn";

            mdBtn.onclick = async () => {
              const res = await fetch(
                `/api/markdowns/view?path=${encodeURIComponent(path)}`
              );
              const mdText = await res.text();
              const html = marked.parse(mdText);
              details.innerHTML = `<h2>${path}</h2>${html}`;
            };

            markdownWrapper.appendChild(mdBtn);
          });

          container.appendChild(markdownWrapper); // ← dodane pod spodem
        }

        // list.appendChild(container);

        btn.onclick = () => {
          details.innerHTML = `
            <h2>${doc.method} ${doc.endpoint}</h2>
            <p><em>${doc.description}</em></p>
            <div><strong>Permissions:</strong> ${doc.permissions}</div>
            <div><strong>Request Body:</strong><pre>${doc.body}</pre></div>
            <div><strong>Request Headers:</strong><pre>${doc.headers}</pre></div>
          `;

          if (doc.query_params && doc.query_params.length > 0) {
            const queryTable = `
              <h3>Query Parameters:</h3>
              <table class="query-params-table">
                <thead>
                  <tr><th>Name</th><th>Value</th></tr>
                </thead>
                <tbody>
                  ${doc.query_params
                    .map(
                      (q) => `
                    <tr>
                      <td>${q.name}</td>
                      <td>${q.value}</td>
                    </tr>
                  `
                    )
                    .join("")}
                </tbody>
              </table>
              <br>
            `;
            details.innerHTML += queryTable;
          }

          const rest = `
           <div><strong>Response:</strong><pre>${doc.res}</pre></div>
            <div><strong>Errors:</strong><ul>
              ${doc.errors
                .map(
                  (e) =>
                    `<li><code>${e.code}</code>: ${e.message} – ${e.description}</li>`
                )
                .join("")}
            </ul></div>`;

          details.innerHTML += rest;

          // Kontener na tabelę testową, jeśli istnieje
          if (doc.defaultDB && doc.defaultDB.length > 0) {
            const headers = Object.keys(doc.defaultDB[0]);

            const tableHTML = `
              <h3>Test Database (DefaultDB)</h3>
              <table class="test-db-table">
                <thead>
                  <tr>
                    ${headers.map((h) => `<th>${h}</th>`).join("")}
                  </tr>
                </thead>
                <tbody>
                  ${doc.defaultDB
                    .map(
                      (row) => `
                    <tr>
                      ${headers.map((h) => `<td>${row[h]}</td>`).join("")}
                    </tr>
                  `
                    )
                    .join("")}
                </tbody>
              </table>
            `;

            details.innerHTML += tableHTML;
          }

          const default_method = doc.method.toUpperCase();
          // req sim part
          details.innerHTML += `
            <h3>Request Simulator</h3>
            <div id="simulator">
              <label for="req-method">Method:</label><br>
               <select id="req-method">
                <option value="GET" ${
                  default_method === "GET" ? "selected" : ""
                }>GET</option>
                <option value="POST" ${
                  default_method === "POST" ? "selected" : ""
                }>POST</option>
                <option value="PUT" ${
                  default_method === "PUT" ? "selected" : ""
                }>PUT</option>
                <option value="DELETE" ${
                  default_method === "DELETE" ? "selected" : ""
                }>DELETE</option>
              </select><br><br>

              <label for="req-headers">Headers (JSON):</label><br>
              <textarea id="req-headers" rows="4" style="width:100%; font-family: monospace;">${
                doc.headers
              }</textarea><br><br>

              <label for="req-body">Body:</label><br>
              <textarea id="req-body" rows="6" style="width:100%; font-family: monospace;">${
                doc.body
              }</textarea><br><br>

              <button id="send-request">Send Request</button>
              <div id="simulator-response" style="margin-top: 1em;"></div>
            </div>
          `;

          document.getElementById("send-request").onclick = async () => {
            const method = document.getElementById("req-method").value;
            const headersRaw = document.getElementById("req-headers").value;
            const body = document.getElementById("req-body").value;
            const resBox = document.getElementById("simulator-response");

            let headers = {};
            try {
              headers = JSON.parse(headersRaw);
            } catch (e) {
              resBox.innerHTML =
                "<span style='color:red;'>Invalid headers JSON</span>";
              return;
            }

            // przygotowanie defaultDB do wysłania (głębokie kopiowanie z doc)
            const dbToSend =
              doc.defaultDB && Array.isArray(doc.defaultDB)
                ? doc.defaultDB.map((entry) => ({ ...entry }))
                : [];

            resBox.innerHTML = "<em>Sending request to Lua...</em>";

            try {
              const res = await fetch("/api/simulate", {
                method: "POST",
                headers: {
                  "Content-Type": "application/json",
                },
                body: JSON.stringify({
                  endpoint: doc.endpoint,
                  method,
                  headers,
                  body,
                  defaultDB: dbToSend,
                }),
              });

              const json = await res.json();
              const logArray = Array.isArray(json.log)
                ? json.log
                : Object.values(json.log || {});

              resBox.innerHTML = `
                <strong>Status:</strong> ${json.response?.status || "N/A"}<br>
                <strong>Response:</strong><pre>${
                  typeof json.response.body === "string"
                    ? json.response.body
                    : JSON.stringify(json.response.body, null, 2)
                }</pre>
                <strong>Log:</strong><pre>${logArray.join("\n")}</pre>
              `;

              if (json.db) {
                // Konwersja: jeśli db jest obiektem (np. {"1": {...}, "2": {...}}), przekształć do tablicy
                const dbArray = Array.isArray(json.db)
                  ? json.db
                  : Object.values(json.db);

                if (dbArray.length === 0) return;

                // Wyciągnij wszystkie unikalne klucze z każdego obiektu (scal kolumny)
                const headersSet = new Set();
                dbArray.forEach((row) => {
                  if (typeof row === "object") {
                    Object.keys(row).forEach((k) => headersSet.add(k));
                  }
                });
                const headers = Array.from(headersSet);

                const tableHTML = `
                  <h4>Updated DB:</h4>
                  <table class="test-db-table">
                    <thead>
                      <tr>${headers.map((h) => `<th>${h}</th>`).join("")}</tr>
                    </thead>
                    <tbody>
                      ${dbArray
                        .map(
                          (row) => `
                        <tr>
                          ${headers
                            .map((h) => `<td>${row[h] ?? ""}</td>`)
                            .join("")}
                        </tr>
                      `
                        )
                        .join("")}
                    </tbody>
                  </table>
                `;

                resBox.innerHTML += tableHTML;
              }
            } catch (err) {
              resBox.innerHTML = `<span style="color:red;"><strong>Error:</strong> ${err}</span>`;
            }
          };
        };
        list.appendChild(container);
      });

      section.appendChild(header);
      section.appendChild(list);
      sidebar.appendChild(section);
    });
  });

fetch(`/api/markdowns?_=${Date.now()}`)
  .then((res) => res.json())
  .then((mdFiles) => {
    const groupedMd = {};
    mdFiles.forEach((file) => {
      if (!groupedMd[file.category]) groupedMd[file.category] = [];
      groupedMd[file.category].push(file);
    });

    Object.entries(groupedMd).forEach(([category, files]) => {
      files = files.filter((file) => !file.name.startsWith("_"));
      if (files.length === 0) return;

      const section = document.createElement("div");
      const header = document.createElement("button");
      header.textContent = `[MD] ${category.toUpperCase()}`;
      header.className = "category-btn";

      const list = document.createElement("div");
      list.style.display = "none";

      header.onclick = () => {
        list.style.display = list.style.display === "none" ? "block" : "none";
      };

      files.forEach((file) => {
        const btn = document.createElement("button");
        btn.className = `endpoint-btn method-md`;
        btn.innerHTML = `
          <span class="method-badge method-md">.md</span>
          <span class="endpoint-text">${file.name}</span>
        `;

        btn.onclick = async () => {
          console.log(encodeURIComponent(file.path));
          const res = await fetch(
            `/api/markdowns/view?path=${encodeURIComponent(file.path)}`
          );
          const mdText = await res.text();

          // Jeśli chcesz renderować jako czysty <pre>:
          // details.innerHTML = `<h2>${file.name}</h2><pre>${mdText}</pre>`;

          // Jeśli chcesz renderować jako HTML (Markdown → HTML):
          const html = marked.parse(mdText);
          details.innerHTML = `<h2>${file.name}</h2>${html}`;

          // Dodaj kolorowanie po załadowaniu
          document.querySelectorAll("pre code").forEach((el) => {
            hljs.highlightElement(el);
          });

          document.querySelectorAll("#details pre code").forEach((block) => {
            hljs.highlightElement(block);
          });
        };

        list.appendChild(btn);
      });

      section.appendChild(header);
      section.appendChild(list);
      document.getElementById("sidebar").appendChild(section);
    });
  });
});