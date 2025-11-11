import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../lib/api'
import { Server } from 'lucide-react'

export default function Brokers() {
  const { data: brokers, isLoading } = useQuery({
    queryKey: ['brokers'],
    queryFn: () => api.listBrokers(),
    refetchInterval: 5000,
  })

  if (isLoading) {
    return <div>Loading...</div>
  }

  return (
    <div>
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900">Brokers</h1>
        <p className="mt-2 text-sm text-gray-600">
          Monitor broker health and status
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {brokers?.map((broker: any) => (
          <Link
            key={broker.id}
            to={`/brokers/${broker.id}`}
            className="card hover:shadow-md transition-shadow"
          >
            <div className="flex items-start space-x-4">
              <div
                className={`p-3 rounded-lg ${
                  broker.status === 'online'
                    ? 'bg-green-100'
                    : 'bg-red-100'
                }`}
              >
                <Server
                  className={`w-6 h-6 ${
                    broker.status === 'online'
                      ? 'text-green-600'
                      : 'text-red-600'
                  }`}
                />
              </div>
              <div className="flex-1">
                <div className="flex items-center justify-between">
                  <div className="text-lg font-semibold">Broker {broker.id}</div>
                  {broker.leader && (
                    <span className="badge badge-info">Leader</span>
                  )}
                </div>
                <div className="mt-2 space-y-1 text-sm text-gray-600">
                  <div>{broker.host}:{broker.port}</div>
                  <div>
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
                  <div>Version: {broker.version}</div>
                  <div>Uptime: {broker.uptime}</div>
                </div>
              </div>
            </div>
          </Link>
        ))}
      </div>
    </div>
  )
}
