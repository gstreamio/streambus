import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../lib/api'
import { Plus, Trash2, Database } from 'lucide-react'
import { useState } from 'react'
import { formatBytes, formatNumber } from '../lib/utils'

export default function Topics() {
  const queryClient = useQueryClient()
  const [showCreateModal, setShowCreateModal] = useState(false)
  const { data: topics, isLoading } = useQuery({
    queryKey: ['topics'],
    queryFn: () => api.listTopics(),
    refetchInterval: 5000,
  })

  const deleteMutation = useMutation({
    mutationFn: (name: string) => api.deleteTopic(name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['topics'] })
    },
  })

  if (isLoading) {
    return <div>Loading...</div>
  }

  return (
    <div>
      <div className="mb-8 flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Topics</h1>
          <p className="mt-2 text-sm text-gray-600">
            Manage topics and partitions
          </p>
        </div>
        <button
          onClick={() => setShowCreateModal(true)}
          className="btn-primary flex items-center"
        >
          <Plus className="w-4 h-4 mr-2" />
          Create Topic
        </button>
      </div>

      <div className="grid grid-cols-1 gap-6">
        {topics?.map((topic: any) => (
          <div key={topic.name} className="card">
            <div className="flex items-start justify-between">
              <div className="flex items-start space-x-4">
                <div className="p-3 bg-blue-100 rounded-lg">
                  <Database className="w-6 h-6 text-blue-600" />
                </div>
                <div>
                  <Link
                    to={`/topics/${topic.name}`}
                    className="text-lg font-semibold text-gray-900 hover:text-primary-600"
                  >
                    {topic.name}
                  </Link>
                  <div className="mt-2 flex items-center space-x-4 text-sm text-gray-600">
                    <span>{topic.partitions} partitions</span>
                    <span>•</span>
                    <span>Replication: {topic.replicationFactor}</span>
                    <span>•</span>
                    <span>{formatNumber(topic.messageCount)} messages</span>
                    <span>•</span>
                    <span>{formatBytes(topic.bytesCount)}</span>
                  </div>
                  <div className="mt-2 flex flex-wrap gap-2">
                    {Object.entries(topic.config || {}).map(([key, value]) => (
                      <span
                        key={key}
                        className="inline-flex items-center px-2 py-1 text-xs bg-gray-100 text-gray-700 rounded"
                      >
                        {key}: {value}
                      </span>
                    ))}
                  </div>
                </div>
              </div>
              <button
                onClick={() => {
                  if (confirm(`Delete topic ${topic.name}?`)) {
                    deleteMutation.mutate(topic.name)
                  }
                }}
                className="btn-danger flex items-center"
              >
                <Trash2 className="w-4 h-4 mr-2" />
                Delete
              </button>
            </div>
          </div>
        ))}
      </div>

      {showCreateModal && (
        <CreateTopicModal
          onClose={() => setShowCreateModal(false)}
          onCreate={() => {
            setShowCreateModal(false)
            queryClient.invalidateQueries({ queryKey: ['topics'] })
          }}
        />
      )}
    </div>
  )
}

function CreateTopicModal({ onClose, onCreate }: any) {
  const [formData, setFormData] = useState({
    name: '',
    partitions: 10,
    replicationFactor: 3,
  })

  const createMutation = useMutation({
    mutationFn: (data: any) => api.createTopic(data),
    onSuccess: onCreate,
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    createMutation.mutate(formData)
  }

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg p-6 max-w-md w-full">
        <h2 className="text-xl font-semibold mb-4">Create New Topic</h2>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Topic Name
            </label>
            <input
              type="text"
              required
              className="input"
              value={formData.name}
              onChange={(e) =>
                setFormData({ ...formData, name: e.target.value })
              }
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Partitions
            </label>
            <input
              type="number"
              required
              min="1"
              className="input"
              value={formData.partitions}
              onChange={(e) =>
                setFormData({ ...formData, partitions: parseInt(e.target.value) })
              }
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Replication Factor
            </label>
            <input
              type="number"
              required
              min="1"
              className="input"
              value={formData.replicationFactor}
              onChange={(e) =>
                setFormData({
                  ...formData,
                  replicationFactor: parseInt(e.target.value),
                })
              }
            />
          </div>
          <div className="flex justify-end space-x-3 pt-4">
            <button type="button" onClick={onClose} className="btn-secondary">
              Cancel
            </button>
            <button
              type="submit"
              disabled={createMutation.isPending}
              className="btn-primary"
            >
              {createMutation.isPending ? 'Creating...' : 'Create'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
