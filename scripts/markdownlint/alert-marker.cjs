// GitHub renders an alert (`> [!NOTE]`, `> [!WARNING]`, ...) only when the
// marker stands alone on the first line of the blockquote. Text on the same
// line turns the alert into a plain quote. The fix moves the text to the next
// line of the blockquote.
const marker = /^(\s*>\s*)\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]([ \t]+)\S/;

module.exports = {
  names: ["github-alert-marker"],
  description: "GitHub alert marker must be alone on its line",
  tags: ["blockquote", "github"],
  parser: "micromark",
  function: (params, onError) => {
    // Skip lines inside code blocks, which may show the syntax.
    const code = new Set();
    for (const token of params.parsers.micromark.tokens) {
      if (token.type === "codeFenced" || token.type === "codeIndented") {
        for (let l = token.startLine; l <= token.endLine; l++) code.add(l);
      }
    }
    params.lines.forEach((line, i) => {
      const m = marker.exec(line);
      if (!m || code.has(i + 1)) return;
      const end = m[1].length + m[2].length + 3; // after "[!NAME]"
      onError({
        lineNumber: i + 1,
        detail: "Put the text of the alert on the next line",
        context: line.slice(0, end),
        fixInfo: {
          editColumn: end + 1,
          deleteCount: m[3].length,
          insertText: "\n" + m[1].replace(/\s+$/, " "),
        },
      });
    });
  },
};
