import React, { useEffect, useState } from 'react';
import { Card, List, Avatar, message, Button, Modal, Form, Input, Row, Col } from 'antd';
import api from '../../utils/api';

const SwimGroupsManagement: React.FC = () => {
  const [groups, setGroups] = useState([]);
  const [selectedGroup, setSelectedGroup] = useState(null);
  const [addChildModalVisible, setAddChildModalVisible] = useState(false);
  const [form] = Form.useForm();

  useEffect(() => {
    fetchGroups();
  }, []);

  const fetchGroups = async () => {
    try {
      const response = await api.get('/admin/swim-groups');
      setGroups(response.data);
    } catch (error) {
      message.error('Failed to fetch swim groups');
    }
  };

  const handleAddChild = async (values: any) => {
    try {
      await api.post(`/admin/swim-groups/${selectedGroup.id}/add-child`, values);
      message.success('Child added successfully');
      setAddChildModalVisible(false);
      fetchGroups();
    } catch (error) {
      message.error('Failed to add child');
    }
  };

  const openAddChildModal = (group: any) => {
    setSelectedGroup(group);
    setAddChildModalVisible(true);
  };

  return (
    <div style={{ padding: '24px' }}>
      <Row gutter={[16, 16]}>
        {groups.map((group: any) => (
          <Col span={24} key={group.id}>
            <Card title={group.name}>
              <p>Coach: {group.coachName}</p>
              <p>Email: {group.coachEmail}</p>
              <p>Practice Schedule: {group.practiceSchedule}</p>
              <List
                itemLayout="horizontal"
                dataSource={group.children}
                renderItem={(child: any) => (
                  <List.Item>
                    <List.Item.Meta
                      avatar={<Avatar src={child.avatar} />}
                      title={child.name}
                      description={`Age: ${child.age} | Status: ${child.status}`}
                    />
                  </List.Item>
                )}
              />
              <Button type="primary" onClick={() => openAddChildModal(group)}>
                Add Child
              </Button>
            </Card>
          </Col>
        ))}
      </Row>

      <Modal
        title="Add Child to Group"
        visible={addChildModalVisible}
        onCancel={() => setAddChildModalVisible(false)}
        footer={null}
      >
        <Form form={form} layout="vertical" onFinish={handleAddChild}>
          <Form.Item
            label="Child Name"
            name="childName"
            rules={[{ required: true, message: 'Please input the child\'s name!' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            label="Age"
            name="age"
            rules={[{ required: true, message: 'Please input the child\'s age!' }]}
          >
            <Input type="number" />
          </Form.Item>
          <Form.Item
            label="Status"
            name="status"
            rules={[{ required: true, message: 'Please input the child\'s status!' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit">
              Add Child
            </Button>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default SwimGroupsManagement;