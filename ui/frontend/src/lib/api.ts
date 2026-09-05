import axios from 'axios'

const baseURL = import.meta.env.VITE_API_URL || '/api'

const client = axios.create({
  baseURL,
  headers: {
    'Content-Type': 'application/json',
  },
})

// Cluster/broker response shapes, mirrored field-for-field from the Go
// structs in ui/backend/services/broker_service.go (see their `json` tags).
// GET /cluster serves ClusterInfo directly; GET /cluster/brokers serves
// []BrokerInfo directly; GET /cluster/health wraps the health string in an
// object rather than returning it bare, so ClusterHealth mirrors that
// wrapper rather than the bare string.
export interface ClusterInfo {
  cluster_id: string
  controller_id: number
  version: string
  total_brokers: number
  active_brokers: number
  total_topics: number
  total_partitions: number
  uptime: string
}

export interface BrokerInfo {
  id: number
  host: string
  port: number
  status: string
  leader: boolean
  version: string
  uptime: string
}

export interface ClusterHealth {
  status: string
}

export const api = {
  // Cluster
  getClusterInfo: (): Promise<ClusterInfo> => client.get('/cluster').then(res => res.data),
  getClusterHealth: (): Promise<ClusterHealth> => client.get('/cluster/health').then(res => res.data),
  listBrokers: (): Promise<BrokerInfo[]> => client.get('/cluster/brokers').then(res => res.data),
  getBroker: (id: number) => client.get(`/cluster/brokers/${id}`).then(res => res.data),

  // Topics
  listTopics: () => client.get('/topics').then(res => res.data),
  getTopic: (name: string) => client.get(`/topics/${name}`).then(res => res.data),
  createTopic: (data: any) => client.post('/topics', data).then(res => res.data),
  deleteTopic: (name: string) => client.delete(`/topics/${name}`).then(res => res.data),
  updateTopicConfig: (name: string, config: any) =>
    client.put(`/topics/${name}/config`, { config }).then(res => res.data),
  getPartitions: (name: string) =>
    client.get(`/topics/${name}/partitions`).then(res => res.data),
  getMessages: (name: string, partition?: number, offset?: number, limit?: number) =>
    client.get(`/topics/${name}/messages`, {
      params: { partition, offset, limit },
    }).then(res => res.data),

  // Consumer Groups
  listConsumerGroups: () => client.get('/consumer-groups').then(res => res.data),
  getConsumerGroup: (id: string) =>
    client.get(`/consumer-groups/${id}`).then(res => res.data),
  getConsumerGroupLag: (id: string) =>
    client.get(`/consumer-groups/${id}/lag`).then(res => res.data),
  resetOffset: (id: string, data: any) =>
    client.post(`/consumer-groups/${id}/reset-offset`, data).then(res => res.data),

  // Metrics
  getThroughput: (duration = '1h') =>
    client.get('/metrics/throughput', { params: { duration } }).then(res => res.data),
  getLatency: (duration = '1h') =>
    client.get('/metrics/latency', { params: { duration } }).then(res => res.data),
  getResources: (duration = '1h') =>
    client.get('/metrics/resources', { params: { duration } }).then(res => res.data),
}

export default client
