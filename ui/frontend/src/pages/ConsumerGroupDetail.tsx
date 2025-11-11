import { useQuery } from '@tanstack/react-query'
import { useParams, Link } from 'react-router-dom'
import { api } from '../lib/api'
import { ArrowLeft, Users, AlertCircle } from 'lucide-react'
import { formatNumber } from '../lib/utils'

export default function ConsumerGroupDetail() {
  const { id } = useParams<{ id: string }>()

  const { data: group } = useQuery({
    queryKey: ['consumer-group', id],
    queryFn: () => api.getConsumerGroup(id!),
    enabled: !!id,
    refetchInterval: 5000,
  })

  const { data: lag } = useQuery({
    queryKey: ['consumer-group-lag', id],
    queryFn: () => api.getConsumerGroupLag(id!),
    enabled: !!id,
    refetchInterval: 5000,
  })

  if (!group) return <div>Loading...</div>

  return (
    <div>
      <div className="mb-6">
        <Link
          to="/consumer-groups"
          className="inline-flex items-center text-sm text-gray-600 hover:text-gray-900"
        >
          <ArrowLeft className="w-4 h-4 mr-1" />
          Back to Consumer Groups
        </Link>
      </div>

      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900">{group.id}</h1>
        <p className="mt-2 text-sm text-gray-600">
          Consumer group details and lag monitoring
        </p>
      </div>

      {/* Overview */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div className="card">
          <div className="text-sm text-gray-600">State</div>
          <span
            className={`badge mt-2 ${
              group.state === 'Stable' ? 'badge-success' : 'badge-warning'
            }`}
          >
            {group.state}
          </span>
        </div>
        <div className="card">
          <div className="text-sm text-gray-600">Members</div>
          <div className="text-2xl font-semibold">{group.members.length}</div>
        </div>
        <div className="card">
          <div className="text-sm text-gray-600">Total Lag</div>
          <div className="text-2xl font-semibold">{formatNumber(group.lag)}</div>
        </div>
        <div className="card">
          <div className="text-sm text-gray-600">Coordinator</div>
          <div className="text-2xl font-semibold">{group.coordinator}</div>
        </div>
      </div>

      {/* Lag by Partition */}
      <div className="card mb-6">
        <h2 className="text-lg font-semibold mb-4">Lag by Partition</h2>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200">
            <thead>
              <tr>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Topic
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Partition
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Member
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Current Offset
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  End Offset
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                  Lag
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200">
              {lag?.map((assignment: any, index: number) => (
                <tr key={index} className="hover:bg-gray-50">
                  <td className="px-4 py-3 text-sm font-medium">
                    {assignment.topic}
                  </td>
                  <td className="px-4 py-3 text-sm">{assignment.partition}</td>
                  <td className="px-4 py-3 text-sm text-gray-600">
                    {assignment.memberId}
                  </td>
                  <td className="px-4 py-3 text-sm">
                    {formatNumber(assignment.offset)}
                  </td>
                  <td className="px-4 py-3 text-sm">
                    {formatNumber(assignment.endOffset)}
                  </td>
                  <td className="px-4 py-3 text-sm">
                    <span
                      className={`badge ${
                        assignment.lag > 1000
                          ? 'badge-warning'
                          : 'badge-success'
                      }`}
                    >
                      {formatNumber(assignment.lag)}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Members */}
      <div className="card">
        <h2 className="text-lg font-semibold mb-4">Members</h2>
        <div className="space-y-4">
          {group.members.map((member: any) => (
            <div
              key={member.id}
              className="border border-gray-200 rounded-lg p-4"
            >
              <div className="flex items-start justify-between">
                <div>
                  <div className="font-semibold">{member.clientId}</div>
                  <div className="text-sm text-gray-600 mt-1">
                    Host: {member.host}
                  </div>
                  <div className="text-sm text-gray-600">
                    Joined: {new Date(member.joinedAt).toLocaleString()}
                  </div>
                  <div className="mt-2">
                    <span className="text-xs text-gray-500">
                      Assigned Partitions:
                    </span>
                    <div className="flex flex-wrap gap-1 mt-1">
                      {member.partitions.map((p: number) => (
                        <span
                          key={p}
                          className="inline-flex items-center px-2 py-0.5 text-xs bg-blue-100 text-blue-800 rounded"
                        >
                          {p}
                        </span>
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
