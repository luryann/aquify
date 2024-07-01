import React, { useState } from 'react';
import { Form, Input, Button, DatePicker, Select, Upload, Card, notification, List } from 'antd';
import { UploadOutlined, PlusOutlined, MinusCircleOutlined } from '@ant-design/icons';
import axios from 'axios';

const { RangePicker } = DatePicker;
const { Option } = Select;

const EventCreator: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [participants, setParticipants] = useState<any[]>([]);
  const [fileList, setFileList] = useState([]);

  const handleFileUpload = (info: any) => {
    let fileList = [...info.fileList];
    fileList = fileList.slice(-1); // Limit to one file
    setFileList(fileList);
  };

  const handleAddParticipant = () => {
    setParticipants([...participants, { key: Date.now(), name: '', role: '' }]);
  };

  const handleRemoveParticipant = (key: number) => {
    setParticipants(participants.filter((participant) => participant.key !== key));
  };

  const handlePublish = async (values: any) => {
    setLoading(true);
    try {
      const formData = new FormData();
      formData.append('title', values.title);
      formData.append('description', values.description);
      formData.append('schedule', JSON.stringify(values.schedule));
      formData.append('participants', JSON.stringify(participants));
      formData.append('file', fileList[0]?.originFileObj || null);
      await axios.post('/api/events', formData);
      notification.success({
        message: 'Event Created',
        description: 'The event has been successfully created.',
      });
      form.resetFields();
      setParticipants([]);
      setFileList([]);
    } catch (error) {
      notification.error({
        message: 'Creation Failed',
        description: 'Failed to create the event.',
      });
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ padding: '24px' }}>
      <Card title="Create Event">
        <Form form={form} layout="vertical" onFinish={handlePublish}>
          <Form.Item
            label="Title"
            name="title"
            rules={[{ required: true, message: 'Please input the title!' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            label="Description"
            name="description"
            rules={[{ required: true, message: 'Please input the description!' }]}
          >
            <Input.TextArea rows={4} />
          </Form.Item>
          <Form.Item label="Schedule">
            <RangePicker showTime />
          </Form.Item>
          <Form.Item label="Upload Meet File">
            <Upload
              fileList={fileList}
              beforeUpload={() => false} // Prevent automatic upload
              onChange={handleFileUpload}
            >
              <Button icon={<UploadOutlined />}>Select File</Button>
            </Upload>
          </Form.Item>
          <Form.Item label="Participants">
            <Button type="dashed" onClick={handleAddParticipant} icon={<PlusOutlined />}>
              Add Participant
            </Button>
            <List
              bordered
              dataSource={participants}
              renderItem={(participant) => (
                <List.Item
                  actions={[
                    <MinusCircleOutlined
                      key="delete"
                      onClick={() => handleRemoveParticipant(participant.key)}
                    />,
                  ]}
                >
                  <Input
                    style={{ marginRight: '8px' }}
                    placeholder="Name"
                    value={participant.name}
                    onChange={(e) =>
                      setParticipants(
                        participants.map((p) =>
                          p.key === participant.key ? { ...p, name: e.target.value } : p
                        )
                      )
                    }
                  />
                  <Select
                    placeholder="Role"
                    style={{ width: '30%' }}
                    value={participant.role}
                    onChange={(value) =>
                      setParticipants(
                        participants.map((p) =>
                          p.key === participant.key ? { ...p, role: value } : p
                        )
                      )
                    }
                  >
                    <Option value="Swimmer">Swimmer</Option>
                    <Option value="Coach">Coach</Option>
                    <Option value="Volunteer">Volunteer</Option>
                  </Select>
                </List.Item>
              )}
            />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading}>
              Create Event
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
};

export default EventCreator;
