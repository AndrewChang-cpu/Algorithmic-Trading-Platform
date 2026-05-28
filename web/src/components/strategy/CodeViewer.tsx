import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { vscDarkPlus } from 'react-syntax-highlighter/dist/esm/styles/prism'

interface Props { code: string }

export default function CodeViewer({ code }: Props) {
  if (!code) return <div style={{ color: '#6e7681', padding: '24px' }}>No code available.</div>

  return (
    <SyntaxHighlighter
      language="python"
      style={vscDarkPlus}
      showLineNumbers
      customStyle={{ background: '#0d1117', margin: 0, fontSize: 13, borderRadius: 6 }}
    >
      {code}
    </SyntaxHighlighter>
  )
}
