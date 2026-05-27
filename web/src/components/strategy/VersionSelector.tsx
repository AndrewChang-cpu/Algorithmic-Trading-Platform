interface Version {
  id: string
  versionNumber: number
  createdAt: string
}

interface VersionSelectorProps {
  versions: Version[]
  selectedId: string
  onSelect: (id: string) => void
}

export default function VersionSelector({ versions, selectedId, onSelect }: VersionSelectorProps) {
  return (
    <select
      value={selectedId}
      onChange={e => onSelect(e.target.value)}
      style={{
        background: '#21262d',
        border: '1px solid #30363d',
        borderRadius: '6px',
        color: '#e6edf3',
        fontSize: '12px',
        padding: '4px 8px',
        cursor: 'pointer',
        outline: 'none',
      }}
    >
      {versions.map((v, i) => (
        <option key={v.id} value={v.id}>
          v{v.versionNumber}{i === 0 ? ' (latest)' : ''}
        </option>
      ))}
    </select>
  )
}
