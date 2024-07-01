import React, { useEffect, useState } from 'react';
import { Card, List, Avatar, message, Row, Col, Typography, Divider, Empty } from 'antd';
import api from '../../utils/api';

const { Title, Text } = Typography;

const SwimGroups: React.FC = () => {
  const [groups, setGroups] = useState([]);
  const [userData, setUserData] = useState<any>(null);

  useEffect(() => {
    fetchUserData();
    fetchGroups();
  }, []);

  const fetchUserData = async () => {
    try {
      const response = await api.get('/user-data');
      setUserData(response.data);
    } catch (error) {
      message.error('Failed to fetch user data');
    }
  };

  const fetchGroups = async () => {
    try {
      const response = await api.get('/user/swim-groups');
      setGroups(response.data);
    } catch (error) {
      message.error('Failed to fetch swim groups');
    }
  };

  const calculateTotalPayment = () => {
    return groups.reduce((total, group) => total + (group.amountDue || 0), 0);
  };

  return (
    <div style={{ padding: '24px' }}>
      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Card title="Account Details">
            {groups.map((group: any) => (
              <Card key={group.id} type="inner" title={`Coach: ${group.coachName}`}>
                <p><Text strong>Group:</Text> {group.name}</p>
                <p><Text strong>Email:</Text> <a href={`mailto:${group.coachEmail}`}>{group.coachEmail}</a></p>
              </Card>
            ))}
          </Card>
        </Col>
        <Col span={24}>
          <Card title="Next Payment">
            {groups.map((group: any) => (
              <p key={group.id}>
                <Text strong>{group.name}:</Text> ${group.amountDue || 0}
              </p>
            ))}
            <Divider />
            <p><Text strong>Total:</Text> ${calculateTotalPayment()}</p>
          </Card>
        </Col>
        <Col span={24}>
          <Card title="Swimmers">
            {groups.length > 0 ? (
              <List
                itemLayout="horizontal"
                dataSource={groups.flatMap(group => group.children)}
                renderItem={(child: any) => (
                  <List.Item>
                    <List.Item.Meta
                      avatar={<Avatar src={child.avatar} />}
                      title={child.name}
                      description={
                        <>
                          <Text>Age: {child.age}</Text>
                          <br />
                          <Text>Status: {child.status}</Text>
                        </>
                      }
                    />
                  </List.Item>
                )}
              />
            ) : (
              <Empty description="No swimmers found" />
            )}
          </Card>
        </Col>
        {groups.map((group: any) => (
          <Col span={24} key={group.id}>
            <Card title={group.name}>
              <p><Text strong>Coach:</Text> {group.coachName}</p>
              <p><Text strong>Email:</Text> <a href={`mailto:${group.coachEmail}`}>{group.coachEmail}</a></p>
              <p><Text strong>Practice Schedule:</Text> {group.practiceSchedule}</p>
              <Divider />
              <Title level={4}>Swimmers in {group.name}</Title>
              <List
                itemLayout="horizontal"
                dataSource={group.children}
                renderItem={(child: any) => (
                  <List.Item>
                    <List.Item.Meta
                      avatar={<Avatar src={child.avatar} />}
                      title={child.name}
                      description={
                        <>
                          <Text>Age: {child.age}</Text>
                          <br />
                          <Text>Status: {child.status}</Text>
                        </>
                      }
                    />
                  </List.Item>
                )}
              />
            </Card>
          </Col>
        ))}
      </Row>
    </div>
  );
};

export default SwimGroups;
