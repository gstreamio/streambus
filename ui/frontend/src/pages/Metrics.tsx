import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import {
  LineChart,
  Line,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts'
import { formatNumber, formatBytes } from '../lib/utils'

export default function Metrics() {
  const { data: throughput } = useQuery({
    queryKey: ['metrics-throughput'],
    queryFn: () => api.getThroughput('1h'),
    refetchInterval: 30000,
  })

  const { data: latency } = useQuery({
    queryKey: ['metrics-latency'],
    queryFn: () => api.getLatency('1h'),
    refetchInterval: 30000,
  })

  const { data: resources } = useQuery({
    queryKey: ['metrics-resources'],
    queryFn: () => api.getResources('1h'),
    refetchInterval: 30000,
  })

  const formatThroughputData = () => {
    if (!throughput) return []
    return throughput.producerRate.map((point: any, index: number) => ({
      timestamp: new Date(point.timestamp).toLocaleTimeString(),
      producer: Math.round(point.value),
      consumer: Math.round(throughput.consumerRate[index]?.value || 0),
    }))
  }

  const formatLatencyData = () => {
    if (!latency) return []
    return latency.p50.map((point: any, index: number) => ({
      timestamp: new Date(point.timestamp).toLocaleTimeString(),
      p50: point.value.toFixed(2),
      p95: latency.p95[index]?.value.toFixed(2) || 0,
      p99: latency.p99[index]?.value.toFixed(2) || 0,
    }))
  }

  const formatResourceData = () => {
    if (!resources) return []
    return resources.cpu.map((point: any, index: number) => ({
      timestamp: new Date(point.timestamp).toLocaleTimeString(),
      cpu: point.value.toFixed(1),
      memory: ((resources.memory[index]?.value || 0) / 1024 / 1024 / 1024).toFixed(
        1
      ),
    }))
  }

  return (
    <div>
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900">Metrics</h1>
        <p className="mt-2 text-sm text-gray-600">
          Real-time performance metrics and monitoring
        </p>
      </div>

      {/* Throughput */}
      <div className="card mb-6">
        <h2 className="text-lg font-semibold mb-4">
          Throughput (Messages/Second)
        </h2>
        <ResponsiveContainer width="100%" height={300}>
          <AreaChart data={formatThroughputData()}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="timestamp" />
            <YAxis />
            <Tooltip formatter={(value) => formatNumber(Number(value))} />
            <Legend />
            <Area
              type="monotone"
              dataKey="producer"
              stackId="1"
              stroke="#3b82f6"
              fill="#3b82f6"
              fillOpacity={0.6}
              name="Producer"
            />
            <Area
              type="monotone"
              dataKey="consumer"
              stackId="1"
              stroke="#10b981"
              fill="#10b981"
              fillOpacity={0.6}
              name="Consumer"
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>

      {/* Latency */}
      <div className="card mb-6">
        <h2 className="text-lg font-semibold mb-4">Latency (Milliseconds)</h2>
        <ResponsiveContainer width="100%" height={300}>
          <LineChart data={formatLatencyData()}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="timestamp" />
            <YAxis />
            <Tooltip />
            <Legend />
            <Line
              type="monotone"
              dataKey="p50"
              stroke="#10b981"
              name="p50"
              dot={false}
            />
            <Line
              type="monotone"
              dataKey="p95"
              stroke="#f59e0b"
              name="p95"
              dot={false}
            />
            <Line
              type="monotone"
              dataKey="p99"
              stroke="#ef4444"
              name="p99"
              dot={false}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>

      {/* Resources */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="card">
          <h2 className="text-lg font-semibold mb-4">CPU Usage (%)</h2>
          <ResponsiveContainer width="100%" height={200}>
            <AreaChart data={formatResourceData()}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="timestamp" />
              <YAxis />
              <Tooltip />
              <Area
                type="monotone"
                dataKey="cpu"
                stroke="#8b5cf6"
                fill="#8b5cf6"
                fillOpacity={0.6}
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>

        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Memory Usage (GB)</h2>
          <ResponsiveContainer width="100%" height={200}>
            <AreaChart data={formatResourceData()}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="timestamp" />
              <YAxis />
              <Tooltip />
              <Area
                type="monotone"
                dataKey="memory"
                stroke="#f59e0b"
                fill="#f59e0b"
                fillOpacity={0.6}
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>
    </div>
  )
}
