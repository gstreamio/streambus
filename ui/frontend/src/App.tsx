import { Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Topics from './pages/Topics'
import TopicDetail from './pages/TopicDetail'
import ConsumerGroups from './pages/ConsumerGroups'
import ConsumerGroupDetail from './pages/ConsumerGroupDetail'
import Brokers from './pages/Brokers'
import BrokerDetail from './pages/BrokerDetail'
import Metrics from './pages/Metrics'

function App() {
  return (
    <Routes>
      <Route path="/" element={<Layout />}>
        <Route index element={<Dashboard />} />
        <Route path="topics" element={<Topics />} />
        <Route path="topics/:name" element={<TopicDetail />} />
        <Route path="consumer-groups" element={<ConsumerGroups />} />
        <Route path="consumer-groups/:id" element={<ConsumerGroupDetail />} />
        <Route path="brokers" element={<Brokers />} />
        <Route path="brokers/:id" element={<BrokerDetail />} />
        <Route path="metrics" element={<Metrics />} />
      </Route>
    </Routes>
  )
}

export default App
