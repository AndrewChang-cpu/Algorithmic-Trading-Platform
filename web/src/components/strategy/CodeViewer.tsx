interface CodeViewerProps {
  code: string
}

const KEYWORDS = ['def', 'class', 'import', 'from', 'return', 'if', 'else', 'elif', 'for', 'while', 'in', 'not', 'and', 'or', 'True', 'False', 'None', 'self', 'pass', 'with', 'as', 'try', 'except', 'finally', 'raise', 'lambda', 'yield']

function highlightLine(line: string): string {
  // Escape HTML
  let result = line
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')

  // Strings (simple: single and double quoted)
  result = result.replace(/(["'])(?:(?!\1)[^\\]|\\.)*\1/g, '<span style="color:#a5d6ff">$&</span>')

  // Comments
  result = result.replace(/(#.*)$/, '<span style="color:#6e7681;font-style:italic">$1</span>')

  // Numbers
  result = result.replace(/\b(\d+\.?\d*)\b/g, '<span style="color:#79c0ff">$1</span>')

  // Keywords
  const kwRe = new RegExp(`\\b(${KEYWORDS.join('|')})\\b`, 'g')
  result = result.replace(kwRe, '<span style="color:#ff7b72">$1</span>')

  return result
}

export default function CodeViewer({ code }: CodeViewerProps) {
  if (!code) return <div style={{ color: '#6e7681', padding: '24px' }}>No code available.</div>

  const lines = code.split('\n')

  return (
    <div style={{
      background: '#0d1117',
      border: '1px solid #21262d',
      borderRadius: '6px',
      overflow: 'auto',
      maxHeight: '600px',
    }}>
      <table style={{ borderCollapse: 'collapse', width: '100%', tableLayout: 'fixed' }}>
        <tbody>
          {lines.map((line, i) => (
            <tr key={i} style={{ lineHeight: '20px' }}>
              <td style={{
                width: '48px', minWidth: '48px',
                padding: '0 12px',
                color: '#484f58',
                fontSize: '12px',
                fontFamily: '"SF Mono", Consolas, monospace',
                textAlign: 'right',
                userSelect: 'none',
                borderRight: '1px solid #21262d',
                verticalAlign: 'top',
              }}>
                {i + 1}
              </td>
              <td style={{
                padding: '0 16px',
                fontFamily: '"SF Mono", Consolas, monospace',
                fontSize: '13px',
                color: '#e6edf3',
                whiteSpace: 'pre',
                overflow: 'visible',
              }}
                dangerouslySetInnerHTML={{ __html: highlightLine(line) || ' ' }}
              />
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
