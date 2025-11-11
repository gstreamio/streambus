import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../lib/api'
import { Users, AlertCircle } from 'lucide-react'
import { formatNumber } from '../lib/utils'

export default function ConsumerGroups() {
  const { data: groups, isLoading } = useQuery({
    queryKey: ['consumer-groups'],
    queryFn: () => api.listConsumerGroups(),
    refetchInterval: 5000,
  })

  if (isLoading) {
    return <div>Loading...</div>
  }

  return (
    <div>
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900">Consumer Groups</h1>
        <p className="mt-2 text-sm text-gray-600">
          Monitor consumer groups and lag
        </p>
      </div>

      <div className="grid grid-cols-1 gap-6">
        {groups?.map((group: any) => (
          <div key={group.id} className="card">
            <div className="flex items-start justify-between">
              <div className="flex items-start space-x-4">
                <div className="p-3 bg-purple-100 rounded-lg">
                  <Users className="w-6 h-6 text-purple-600" />
                </div>
                <div>
                  <Link
                    to={`/consumer-groups/${group.id}`}
                    className="text-lg font-semibold text-gray-900 hover:text-primary-600"
                  >
                    {group.id}
                  </Link>
                  <div className="mt-2 flex items-center space-x-4 text-sm text-gray-600">
                    <span
                      className={`badge ${
                        group.state === 'Stable'
                          ? 'badge-success'
                          : 'badge-warning'
                      }`}
                    >
                      {group.state}
                    </span>
                    <span>{group.members.length} members</span>
                    <span>•</span>
                    <span>Coordinator: {group.coordinator}</span>
                  </div>
                  <div className="mt-3">
                    {group.lag > 1000 && (
                      <div className="flex items-center text-sm text-orange-600">
                        <AlertCircle className="w-4 h-4 mr-1" />
                        <span>High lag: {formatNumber(group.lag)} messages</span>
                      </div>
                    )}
                    {group.lag <= 1000 && (
                      <div className="text-sm text-gray-600">
                        Lag: {formatNumber(group.lag)} messages
                      </div>
                    )}
                  </div>
                  <div className="mt-3">
                    <div className="text-xs text-gray-500 mb-1">Members:</div>
                    <div className="flex flex-wrap gap-2">
                      {group.members.map((member: any) => (
                        <span
                          key={member.id}
                          className="inline-flex items-center px-2 py-1 text-xs bg-gray-100 text-gray-700 rounded"
                        >
                          {member.clientId} ({member.partitions.length} partitions)
                        </span>
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
