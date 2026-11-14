import { useEffect, useState } from 'react'

export default function App() {
  const [health, setHealth] = useState({ state: 'loading', text: 'Checking API...' })

  useEffect(() => {
    let cancelled = false

    fetch('/api/health')
      .then((res) => {
        if (!res.ok) {
          throw new Error(`HTTP ${res.status}`)
        }
        return res.json()
      })
      .then((data) => {
        if (!cancelled) {
          setHealth({ state: 'ok', text: `API status: ${data.status}` })
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setHealth({ state: 'err', text: `API error: ${err.message}` })
        }
      })

    return () => {
      cancelled = true
    }
  }, [])

  return (
    <main className="container">
      <h1>sgai</h1>
      <p>Go + Chi web service with an embedded React JS application.</p>
      <div className={`status ${health.state}`}>{health.text}</div>
    </main>
  )
}
