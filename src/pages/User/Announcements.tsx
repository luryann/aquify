import React, { useEffect, useState } from 'react';
import { Table, Input, Select, Space, Tag, Typography, Divider } from 'antd';
import { SearchOutlined } from '@ant-design/icons';
import axios from 'axios';
import moment from 'moment';

const { Option } = Select;
const { Title, Paragraph } = Typography;

const Announcements: React.FC = () => {
  const [announcements, setAnnouncements] = useState([]);
  const [loading, setLoading] = useState(false);
  const [filters, setFilters] = useState({
    search: '',
    category: '',
  });

  useEffect(() => {
    fetchAnnouncements();
  }, [filters]);

  const fetchAnnouncements = async () => {
    setLoading(true);
    try {
      const response = await axios.get('/api/announcements', { params: filters });
      setAnnouncements(response.data);
    } catch (error) {
      console.error('Failed to fetch announcements', error);
      // Handle error appropriately, maybe with a notification
    } finally {
      setLoading(false);
    }
  };

  const handleFilterChange = (value: any, key: string) => {
    setFilters({ ...filters, [key]: value });
  };

  const columns = [
    {
      title: 'Title',
      dataIndex: 'title',
      key: 'title',
      render: (text: string, record: any) => (
        <div>
          <Title level={5}>{text}</Title>
          <Paragraph ellipsis={{ rows: 2 }}>{record.content}</Paragraph>
        </div>
      ),
    },
    {
      title: 'Date',
      dataIndex: 'date',
      key: 'date',
      render: (date: string) => moment(date).format('YYYY-MM-DD'),
    },
    {
      title: 'Category',
      dataIndex: 'category',
      key: 'category',
    },
    {
      title: 'Tags',
      dataIndex: 'tags',
      key: 'tags',
      render: (tags: string[]) => (
        <>
          {tags.map(tag => (
            <Tag key={tag} color="blue">
              {tag}
            </Tag>
          ))}
        </>
      ),
    },
  ];

  return (
    <div style={{ padding: '24px' }}>
      <Space style={{ marginBottom: '16px' }}>
        <Input
          placeholder="Search for announcements..."
          prefix={<SearchOutlined />}
          onChange={(e) => handleFilterChange(e.target.value, 'search')}
        />
        <Select
          placeholder="Select Category"
          onChange={(value) => handleFilterChange(value, 'category')}
          style={{ width: 150 }}
        >
          <Option value="">All Categories</Option>
          <Option value="General">General</Option>
          <Option value="Events">Events</Option>
          <Option value="Updates">Updates</Option>
        </Select>
      </Space>
      <Divider />
      <Table
        columns={columns}
        dataSource={announcements}
        loading={loading}
        rowKey="id"
        pagination={{ pageSize: 10 }}
      />
    </div>
  );
};

export default Announcements;
