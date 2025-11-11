import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { Activity, Database, Users, TrendingUp } from 'lucide-react'
import { formatNumber, formatBytes } from '../lib/utils'

export default function Dashboard() {
  const { data: cluster } = useQuery({
    queryKey: ['cluster'],
    queryFn: () => api.getClusterInfo(),
    refetchInterval: 5000,
  })

  const { data: topics } = useQuery({
    queryKey: ['topics'],
    queryFn: () => api.listTopics(),
  })

  const { data: groups } = useQuery({
    queryKey: ['consumer-groups'],
    queryFn: () => api.listConsumerGroups(),
  })

  const stats = [
    {
      name: 'Brokers',
      value: cluster?.brokers.length || 0,
      icon: Activity,
      color: 'text-blue-600 bg-blue-100',
      change: cluster?.status === 'healthy' ? 'All Online' : 'Degraded',
    },
    {
      name: 'Topics',
      value: topics?.length || 0,
      icon: Database,
      color: 'text-green-600 bg-green-100',
      change: `${cluster?.partitionCount || 0} partitions`,
    },
    {
      name: 'Consumer Groups',
      value: groups?.length || 0,
      icon: Users,
      color: 'text-purple-600 bg-purple-100',
      change: 'Active',
    },
    {
      name: 'Messages',
      value: formatNumber(1234567),
      icon: TrendingUp,
      color: 'text-orange-600 bg-orange-100',
      change: '+12% from yesterday',
    },
  ]

  return (
    <div>
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900">Dashboard</h1>
        <p className="mt-2 text-sm text-gray-600">
          Overview of your StreamBus cluster
        </p>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-4 mb-8">
        {stats.map((stat) => (
          <div key={stat.name} className="card">
            <div className="flex items-center">
              <div className={`p-3 rounded-lg ${stat.color}`}>
                <stat.icon className="w-6 h-6" />
              </div>
              <div className="ml-4">
                <p className="text-sm font-medium text-gray-600">{stat.name}</p>
                <p className="text-2xl font-semibold text-gray-900">
                  {stat.value}
                </p>
                <p className="text-xs text-gray-500">{stat.change}</p>
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Cluster Status */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Cluster Health</h2>
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-600">Status</span>
              <span className={`badge ${
                cluster?.status === 'healthy'
                  ? 'badge-success'
                  : 'badge-warning'
              }`}>
                {cluster?.status || 'Unknown'}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-600">Cluster ID</span>
              <span className="text-sm font-mono">{cluster?.clusterId}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-600">Online Brokers</span>
              <span className="text-sm font-semibold">
                {cluster?.brokers.filter(b => b.status === 'online').length} / {cluster?.brokers.length}
              </span>
            </div>
          </div>
        </div>

        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Recent Activity</h2>
          <div className="space-y-3">
            <div className="flex items-start">
              <div className="flex-shrink-0 w-2 h-2 mt-2 rounded-full bg-green-500" />
              <div className="ml-3">
                <p className="text-sm text-gray-900">Cluster healthy</p>
                <p className="text-xs text-gray-500">2 minutes ago</p>
              </div>
            </div>
            <div className="flex items-start">
              <div className="flex-shrink-0 w-2 h-2 mt-2 rounded-full bg-blue-500" />
              <div className="ml-3">
                <p className="text-sm text-gray-900">New topic created: orders</p>
                <p className="text-xs text-gray-500">1 hour ago</p>
              </div>
            </div>
            <div className="flex items-start">
              <div className="flex-shrink-0 w-2 h-2 mt-2 rounded-full bg-purple-500" />
              <div className="ml-3">
                <p className="text-sm text-gray-900">Consumer group joined</p>
                <p className="text-xs text-gray-500">3 hours ago</p>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Brokers List */}
      <div className="mt-6">
        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Brokers</h2>
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead>
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    ID
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Host
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Status
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Role
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Uptime
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {cluster?.brokers.map((broker) => (
                  <tr key={broker.id} className="hover:bg-gray-50">
                    <td className="px-4 py-3 text-sm font-medium text-gray-900">
                      {broker.id}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-600">
                      {broker.host}
                    </td>
                    <td className="px-4 py-3 text-sm">
                      <span className={`badge ${
                        broker.status === 'online'
                          ? 'badge-success'
                          : 'badge-danger'
                      }`}>
                        {broker.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm">
                      {broker.leader && (
                        <span className="badge badge-info">Leader</span>
                      )}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-600">
                      {broker.uptime}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  )
}
