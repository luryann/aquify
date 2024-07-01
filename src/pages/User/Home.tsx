import React, { useEffect, useState } from 'react';
import { Card, List, Statistic, Row, Col, Select, Spin } from 'antd';
import { LineChart, Line, XAxis, YAxis, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import axios from 'axios';

const { Option } = Select;

const Home: React.FC = () => {
  const [homeData, setHomeData] = useState<any>(null);
  const [performanceData, setPerformanceData] = useState([]);
  const [selectedEvent, setSelectedEvent] = useState<string | null>(null);
  const [selectedSwimmer, setSelectedSwimmer] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [performanceLoading, setPerformanceLoading] = useState(false);

  useEffect(() => {
    fetchHomeData();
  }, []);

  useEffect(() => {
    if (selectedEvent && selectedSwimmer) {
      fetchPerformanceData(selectedEvent, selectedSwimmer);
    }
  }, [selectedEvent, selectedSwimmer]);

  const fetchHomeData = async () => {
    setLoading(true);
    try {
      const response = await axios.get('/api/home-data');
      setHomeData(response.data);
      setSelectedSwimmer(response.data.swimmers?.[0]?.id || null); // Set default selected swimmer
      setSelectedEvent(response.data.events?.[0]?.id || null); // Set default selected event
    } catch (error) {
      console.error('Failed to fetch home data', error);
    } finally {
      setLoading(false);
    }
  };

  const fetchPerformanceData = async (eventId: string, swimmerId: string) => {
    setPerformanceLoading(true);
    try {
      const response = await axios.get(`/api/performance-data?event=${eventId}&swimmer=${swimmerId}`);
      setPerformanceData(response.data);
    } catch (error) {
      console.error('Failed to fetch performance data', error);
    } finally {
      setPerformanceLoading(false);
    }
  };

  return (
    <div style={{ padding: '24px' }}>
      <Row gutter={16}>
        <Col span={8}>
          <Card title="Account Details" loading={loading}>
            <p>Name: {homeData?.accountDetails?.name}</p>
            <p>Email: {homeData?.accountDetails?.email}</p>
          </Card>
        </Col>
        <Col span={8}>
          <Card title="Next Payment" loading={loading}>
            <Statistic value={homeData?.nextPayment || 0} prefix="$" />
          </Card>
        </Col>
        <Col span={8}>
          <Card title="Swimmers" loading={loading}>
            <List
              dataSource={homeData?.swimmers || []}
              renderItem={(swimmer) => (
                <List.Item>
                  <List.Item.Meta title={swimmer.name} description={`Status: ${swimmer.status}`} />
                </List.Item>
              )}
            />
          </Card>
        </Col>
      </Row>
      <Row gutter={16} style={{ marginTop: '24px' }}>
        <Col span={24}>
          <Card title="Performance Chart" loading={loading}>
            <Select
              style={{ width: 200, marginBottom: '16px', marginRight: '16px' }}
              placeholder="Select Swimmer"
              value={selectedSwimmer}
              onChange={(value) => setSelectedSwimmer(value)}
            >
              {homeData?.swimmers?.map((swimmer: any) => (
                <Option key={swimmer.id} value={swimmer.id}>
                  {swimmer.name}
                </Option>
              ))}
            </Select>
            <Select
              style={{ width: 200, marginBottom: '16px' }}
              placeholder="Select Event"
              value={selectedEvent}
              onChange={(value) => setSelectedEvent(value)}
            >
              {homeData?.events?.map((event: any) => (
                <Option key={event.id} value={event.id}>
                  {event.title}
                </Option>
              ))}
            </Select>
            {performanceLoading ? (
              <Spin />
            ) : (
              <ResponsiveContainer width="100%" height={300}>
                <LineChart data={performanceData}>
                  <XAxis dataKey="name" />
                  <YAxis />
                  <Tooltip />
                  <Legend />
                  <Line type="monotone" dataKey="time" stroke="#8884d8" />
                </LineChart>
              </ResponsiveContainer>
            )}
          </Card>
        </Col>
      </Row>
      <Row gutter={16} style={{ marginTop: '24px' }}>
        <Col span={12}>
          <Card title="Latest Announcements" loading={loading}>
            <List
              dataSource={homeData?.announcements || []}
              renderItem={(announcement) => (
                <List.Item>
                  <List.Item.Meta title={announcement.title} description={announcement.description} />
                </List.Item>
              )}
            />
          </Card>
        </Col>
        <Col span={12}>
          <Card title="Upcoming Events" loading={loading}>
            <List
              dataSource={homeData?.events || []}
              renderItem={(event) => (
                <List.Item>
                  <List.Item.Meta title={event.title} description={event.date} />
                </List.Item>
              )}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default Home;
