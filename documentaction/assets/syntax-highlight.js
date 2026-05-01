(function () {
  function escapeHTML(value) {
    return value
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
  }

  function tokenize(source, rules) {
    var tokens = [];
    var out = source;

    rules.forEach(function (rule) {
      out = out.replace(rule.pattern, function () {
        var match = arguments[0];
        var rendered = rule.render(match);
        var id = String.fromCharCode(0xE000 + tokens.length);
        tokens.push(rendered);
        return id;
      });
    });

    out = escapeHTML(out);

    tokens.forEach(function (token, index) {
      var id = String.fromCharCode(0xE000 + index);
      out = out.split(id).join(token);
    });

    return out;
  }

  function span(className, value) {
    return '<span class="token ' + className + '">' + escapeHTML(value) + "</span>";
  }

  function spanEscaped(className, value) {
    return '<span class="token ' + className + '">' + value + "</span>";
  }

  function highlightGo(source) {
    var keywords = "break default func interface select case defer go map struct chan else goto package switch const fallthrough if range type continue for import return var";
    var types = "any bool byte comparable complex64 complex128 error float32 float64 int int8 int16 int32 int64 rune string uint uint8 uint16 uint32 uint64 uintptr";
    var builtins = "append cap close complex copy delete imag len make new panic print println real recover";

    return tokenize(source, [
      { pattern: /`[\s\S]*?`/g, render: function (m) { return span("string", m); } },
      { pattern: /"(?:\\.|[^"\\])*"/g, render: function (m) { return span("string", m); } },
      { pattern: /'(?:\\.|[^'\\])*'/g, render: function (m) { return span("string", m); } },
      { pattern: /\/\/[^\n]*/g, render: function (m) { return span("comment", m); } },
      { pattern: /\/\*[\s\S]*?\*\//g, render: function (m) { return span("comment", m); } },
      { pattern: /\b\d+(?:\.\d+)?\b/g, render: function (m) { return span("number", m); } },
      { pattern: new RegExp("\\b(" + keywords.replace(/ /g, "|") + ")\\b", "g"), render: function (m) { return span("keyword", m); } },
      { pattern: new RegExp("\\b(" + types.replace(/ /g, "|") + ")\\b", "g"), render: function (m) { return span("type", m); } },
      { pattern: new RegExp("\\b(" + builtins.replace(/ /g, "|") + ")\\b(?=\\()", "g"), render: function (m) { return span("function", m); } },
      { pattern: /\b[A-Za-z_]\w*(?=\()/g, render: function (m) { return span("function", m); } },
      { pattern: /[{}[\]();,.]/g, render: function (m) { return span("punctuation", m); } },
      { pattern: /[-+*/%=!<>:&|]+/g, render: function (m) { return span("operator", m); } }
    ]);
  }

  function highlightJSON(source) {
    return tokenize(source, [
      { pattern: /"(?:\\.|[^"\\])*"(?=\s*:)/g, render: function (m) { return span("property", m); } },
      { pattern: /"(?:\\.|[^"\\])*"/g, render: function (m) { return span("string", m); } },
      { pattern: /\b-?\d+(?:\.\d+)?(?:e[+-]?\d+)?\b/gi, render: function (m) { return span("number", m); } },
      { pattern: /\b(?:true|false)\b/g, render: function (m) { return span("boolean", m); } },
      { pattern: /\bnull\b/g, render: function (m) { return span("null", m); } },
      { pattern: /[{}[\]:,]/g, render: function (m) { return span("punctuation", m); } }
    ]);
  }

  function highlightBash(source) {
    return tokenize(source, [
      { pattern: /"(?:\\.|[^"\\])*"/g, render: function (m) { return span("string", m); } },
      { pattern: /'(?:\\.|[^'\\])*'/g, render: function (m) { return span("string", m); } },
      { pattern: /#[^\n]*/g, render: function (m) { return span("comment", m); } },
      { pattern: /^\s*(curl|go|TsunamiDB\.exe|\.\/TsunamiDB-linux|http|wget)\b/gm, render: function (m) { return span("command", m); } },
      { pattern: /\s-{1,2}[A-Za-z0-9][A-Za-z0-9-]*/g, render: function (m) { return span("flag", m); } },
      { pattern: /\b\d+(?:\.\d+)?\b/g, render: function (m) { return span("number", m); } },
      { pattern: /[|&;\\]/g, render: function (m) { return span("operator", m); } }
    ]);
  }

  function highlightText(source) {
    return escapeHTML(source)
      .replace(/^([A-Za-z][A-Za-z ]+ -&gt; [A-Za-z][A-Za-z ]+)/gm, function (m) {
        return spanEscaped("command", m);
      })
      .replace(/(POST|GET|WS|Body|Response|Send|Connect|Stream updates):/g, function (m) {
        return spanEscaped("keyword", m);
      });
  }

  function languageOf(block) {
    var match = (block.className || "").match(/language-([a-z0-9-]+)/i);
    return match ? match[1].toLowerCase() : "";
  }

  document.addEventListener("DOMContentLoaded", function () {
    document.querySelectorAll("pre code").forEach(function (block) {
      var lang = languageOf(block);
      var source = block.textContent;

      if (lang === "go") {
        block.innerHTML = highlightGo(source);
      } else if (lang === "json") {
        block.innerHTML = highlightJSON(source);
      } else if (lang === "bash" || lang === "shell") {
        block.innerHTML = highlightBash(source);
      } else if (lang === "text") {
        block.innerHTML = highlightText(source);
      } else {
        block.innerHTML = escapeHTML(source);
      }
    });
  });
})();
