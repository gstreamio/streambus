import { useQuery } from '@tanstack/react-query'
import { useParams, Link } from 'react-router-dom'
import { api } from '../lib/api'
import { ArrowLeft, Server } from 'lucide-react'

export default function BrokerDetail() {
  const { id } = useParams<{ id: string }>()

  const { data: broker } = useQuery({
    queryKey: ['broker', id],
    queryFn: () => api.getBroker(parseInt(id!)),
    enabled: !!id,
    refetchInterval: 5000,
  })

  if (!broker) return <div>Loading...</div>

  return (
    <div>
      <div className="mb-6">
        <Link
          to="/brokers"
          className="inline-flex items-center text-sm text-gray-600 hover:text-gray-900"
        >
          <ArrowLeft className="w-4 h-4 mr-1" />
          Back to Brokers
        </Link>
      </div>

      <div className="mb-8">
        <div className="flex items-center space-x-3">
          <Server className="w-8 h-8 text-gray-600" />
          <h1 className="text-3xl font-bold text-gray-900">
            Broker {broker.id}
          </h1>
          {broker.leader && <span className="badge badge-info">Leader</span>}
          <span
            className={`badge ${
              broker.status === 'online' ? 'badge-success' : 'badge-danger'
            }`}
          >
            {broker.status}
          </span>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Broker Information</h2>
          <div className="space-y-3">
            <div className="flex justify-between">
              <span className="text-gray-600">ID</span>
              <span className="font-semibold">{broker.id}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-600">Host</span>
              <span className="font-mono text-sm">{broker.host}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-600">Port</span>
              <span className="font-mono text-sm">{broker.port}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-600">Version</span>
              <span className="font-mono text-sm">{broker.version}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-600">Status</span>
              <span
                className={`badge ${
                  broker.status === 'online'
                    ? 'badge-success'
                    : 'badge-danger'
                }`}
              >
                {broker.status}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-600">Uptime</span>
              <span>{broker.uptime}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-600">Role</span>
              <span>{broker.leader ? 'Leader' : 'Follower'}</span>
            </div>
          </div>
        </div>

        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Metrics</h2>
          <div className="space-y-3">
            <div>
              <div className="text-sm text-gray-600 mb-1">CPU Usage</div>
              <div className="w-full bg-gray-200 rounded-full h-2">
                <div
                  className="bg-blue-600 h-2 rounded-full"
                  style={{ width: '45%' }}
                />
              </div>
              <div className="text-xs text-gray-500 mt-1">45%</div>
            </div>
            <div>
              <div className="text-sm text-gray-600 mb-1">Memory Usage</div>
              <div className="w-full bg-gray-200 rounded-full h-2">
                <div
                  className="bg-green-600 h-2 rounded-full"
                  style={{ width: '60%' }}
                />
              </div>
              <div className="text-xs text-gray-500 mt-1">60%</div>
            </div>
            <div>
              <div className="text-sm text-gray-600 mb-1">Disk Usage</div>
              <div className="w-full bg-gray-200 rounded-full h-2">
                <div
                  className="bg-yellow-600 h-2 rounded-full"
                  style={{ width: '35%' }}
                />
              </div>
              <div className="text-xs text-gray-500 mt-1">35%</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
