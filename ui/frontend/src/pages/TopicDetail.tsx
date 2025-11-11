import { useQuery } from '@tanstack/react-query'
import { useParams, Link } from 'react-router-dom'
import { api } from '../lib/api'
import { ArrowLeft, Database, HardDrive } from 'lucide-react'
import { formatBytes, formatNumber } from '../lib/utils'

export default function TopicDetail() {
  const { name } = useParams<{ name: string }>()

  const { data: topic } = useQuery({
    queryKey: ['topic', name],
    queryFn: () => api.getTopic(name!),
    enabled: !!name,
  })

  const { data: partitions } = useQuery({
    queryKey: ['partitions', name],
    queryFn: () => api.getPartitions(name!),
    enabled: !!name,
  })

  const { data: messages } = useQuery({
    queryKey: ['messages', name],
    queryFn: () => api.getMessages(name!, 0, 0, 50),
    enabled: !!name,
  })

  if (!topic) return <div>Loading...</div>

  return (
    <div>
      <div className="mb-6">
        <Link
          to="/topics"
          className="inline-flex items-center text-sm text-gray-600 hover:text-gray-900"
        >
          <ArrowLeft className="w-4 h-4 mr-1" />
          Back to Topics
        </Link>
      </div>

      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900">{topic.name}</h1>
        <p className="mt-2 text-sm text-gray-600">Topic details and partitions</p>
      </div>

      {/* Overview */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div className="card">
          <div className="text-sm text-gray-600">Partitions</div>
          <div className="text-2xl font-semibold">{topic.partitions}</div>
        </div>
        <div className="card">
          <div className="text-sm text-gray-600">Replication</div>
          <div className="text-2xl font-semibold">{topic.replicationFactor}</div>
        </div>
        <div className="card">
          <div className="text-sm text-gray-600">Messages</div>
          <div className="text-2xl font-semibold">
            {formatNumber(topic.messageCount)}
          </div>
        </div>
        <div className="card">
          <div className="text-sm text-gray-600">Size</div>
          <div className="text-2xl font-semibold">
            {formatBytes(topic.bytesCount)}
          </div>
        </div>
      </div>

      {/* Partitions */}
      <div className="card mb-6">
        <h2 className="text-lg font-semibold mb-4">Partitions</h2>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200">
            <thead>
              <tr>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Partition
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Leader
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Replicas
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  ISR
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Messages
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Size
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Status
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200">
              {partitions?.map((partition: any) => (
                <tr key={partition.id} className="hover:bg-gray-50">
                  <td className="px-4 py-3 text-sm font-medium">
                    {partition.id}
                  </td>
                  <td className="px-4 py-3 text-sm">{partition.leader}</td>
                  <td className="px-4 py-3 text-sm">
                    {partition.replicas.join(', ')}
                  </td>
                  <td className="px-4 py-3 text-sm">
                    {partition.isr.join(', ')}
                  </td>
                  <td className="px-4 py-3 text-sm">
                    {formatNumber(partition.messages)}
                  </td>
                  <td className="px-4 py-3 text-sm">
                    {formatBytes(partition.size)}
                  </td>
                  <td className="px-4 py-3 text-sm">
                    <span
                      className={`badge ${
                        partition.status === 'healthy'
                          ? 'badge-success'
                          : 'badge-warning'
                      }`}
                    >
                      {partition.status}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Recent Messages */}
      <div className="card">
        <h2 className="text-lg font-semibold mb-4">Recent Messages</h2>
        <div className="space-y-3">
          {messages?.map((message: any) => (
            <div
              key={message.offset}
              className="border border-gray-200 rounded-lg p-4"
            >
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center space-x-4 text-sm text-gray-600">
                  <span>Offset: {message.offset}</span>
                  <span>Partition: {message.partition}</span>
                  <span>{new Date(message.timestamp).toLocaleString()}</span>
                </div>
              </div>
              <div className="mb-2">
                <span className="text-xs text-gray-500">Key:</span>
                <div className="font-mono text-sm bg-gray-50 p-2 rounded mt-1">
                  {message.key || '(null)'}
                </div>
              </div>
              <div>
                <span className="text-xs text-gray-500">Value:</span>
                <div className="font-mono text-sm bg-gray-50 p-2 rounded mt-1">
                  {message.value}
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
