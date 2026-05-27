import { useEffect, useRef } from 'react'
import { createChart, IChartApi, ISeriesApi, UTCTimestamp } from 'lightweight-charts'

interface OHLCPoint {
  time: number
  open: number
  high: number
  low: number
  close: number
}

export default function EquityCurve({ data }: { data: OHLCPoint[] }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<IChartApi | null>(null)
  const seriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null)

  useEffect(() => {
    if (!containerRef.current) return

    const chart = createChart(containerRef.current, {
      width: containerRef.current.clientWidth,
      height: 300,
      layout: {
        background: { color: '#0d1117' },
        textColor: '#6e7681',
      },
      grid: {
        vertLines: { color: '#21262d' },
        horzLines: { color: '#21262d' },
      },
      crosshair: { mode: 1 },
      timeScale: { borderColor: '#30363d' },
      rightPriceScale: { borderColor: '#30363d' },
    })

    const series = chart.addCandlestickSeries({
      upColor: '#3fb950',
      downColor: '#f85149',
      borderUpColor: '#3fb950',
      borderDownColor: '#f85149',
      wickUpColor: '#3fb950',
      wickDownColor: '#f85149',
    })

    chartRef.current = chart
    seriesRef.current = series

    const ro = new ResizeObserver(() => {
      if (containerRef.current) {
        chart.applyOptions({ width: containerRef.current.clientWidth })
      }
    })
    ro.observe(containerRef.current)

    return () => {
      ro.disconnect()
      chart.remove()
    }
  }, [])

  useEffect(() => {
    if (!seriesRef.current || !data.length) return
    seriesRef.current.setData(
      data.map(p => ({ ...p, time: p.time as UTCTimestamp }))
    )
    chartRef.current?.timeScale().fitContent()
  }, [data])

  return (
    <div
      ref={containerRef}
      style={{
        width: '100%',
        background: '#0d1117',
        border: '1px solid #21262d',
        borderRadius: '6px',
        overflow: 'hidden',
      }}
    />
  )
}
