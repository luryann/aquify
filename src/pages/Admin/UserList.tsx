import React, { useEffect, useState } from 'react';
import { Table, Button, Input, Select, Space, Modal, Tag, notification } from 'antd';
import { SearchOutlined, FileExcelOutlined, ExportOutlined } from '@ant-design/icons';
import axios from 'axios';
import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';

const { Option } = Select;

const UserList: React.FC = () => {
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedUsers, setSelectedUsers] = useState([]);
  const [filters, setFilters] = useState({
    search: '',
    gender: '',
    ageGroup: '',
    status: '',
  });

  useEffect(() => {
    fetchUsers();
  }, [filters]);

  const fetchUsers = async () => {
    setLoading(true);
    try {
      const response = await axios.get('/api/users', { params: filters });
      setUsers(response.data);
    } catch (error) {
      console.error('Failed to fetch users', error);
    } finally {
      setLoading(false);
    }
  };

  const handleFilterChange = (value: any, key: string) => {
    setFilters({ ...filters, [key]: value });
  };

  const handleBulkDelete = async () => {
    try {
      await axios.post('/api/users/bulk-delete', { userIds: selectedUsers });
      notification.success({
        message: 'Users Deleted',
        description: 'The selected users have been successfully deleted.',
      });
      fetchUsers();
    } catch (error) {
      console.error('Failed to delete users', error);
      notification.error({
        message: 'Deletion Failed',
        description: 'Failed to delete the selected users.',
      });
    }
  };

  const exportToPDF = () => {
    const doc = new jsPDF();
    autoTable(doc, { html: '#userTable' });
    doc.save('user-list.pdf');
  };

  const exportToExcel = () => {
    // Placeholder for Excel export functionality
    notification.info({
      message: 'Export to Excel',
      description: 'Export to Excel functionality is not implemented yet.',
    });
  };

  const columns = [
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: 'Gender',
      dataIndex: 'gender',
      key: 'gender',
      render: (gender: string) => <Tag color={gender === 'Male' ? 'blue' : 'pink'}>{gender}</Tag>,
    },
    {
      title: 'Age',
      dataIndex: 'age',
      key: 'age',
    },
    {
      title: 'Group',
      dataIndex: 'group',
      key: 'group',
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => <Tag color={status === 'Active' ? 'green' : 'red'}>{status}</Tag>,
    },
    {
      title: 'Actions',
      key: 'actions',
      render: (text: any, record: any) => (
        <Space size="middle">
          <Button onClick={() => viewProfile(record)}>View Profile</Button>
          <Button type="primary" onClick={() => editUser(record)}>Edit</Button>
          <Button type="danger" onClick={() => deleteUser(record)}>Delete</Button>
        </Space>
      ),
    },
  ];

  const rowSelection = {
    onChange: (selectedRowKeys: React.Key[], selectedRows: any[]) => {
      setSelectedUsers(selectedRowKeys as string[]);
    },
  };

  const viewProfile = (user: any) => {
    Modal.info({
      title: 'User Profile',
      content: (
        <div>
          <p><strong>Name:</strong> {user.name}</p>
          <p><strong>Gender:</strong> {user.gender}</p>
          <p><strong>Age:</strong> {user.age}</p>
          <p><strong>Group:</strong> {user.group}</p>
          <p><strong>Status:</strong> {user.status}</p>
          <p><strong>Activity Log:</strong></p>
          <ul>
            {user.activityLog.map((log: any, index: number) => (
              <li key={index}>{log}</li>
            ))}
          </ul>
          <p><strong>Performance Data:</strong></p>
          <ul>
            {user.performanceData.map((data: any, index: number) => (
              <li key={index}>{data}</li>
            ))}
          </ul>
          <p><strong>Account History:</strong></p>
          <ul>
            {user.accountHistory.map((history: any, index: number) => (
              <li key={index}>{history}</li>
            ))}
          </ul>
        </div>
      ),
      onOk() {},
    });
  };

  const editUser = (user: any) => {
    // Implement edit user functionality here
    console.log('Editing user:', user);
  };

  const deleteUser = async (user: any) => {
    try {
      await axios.delete(`/api/users/${user.id}`);
      notification.success({
        message: 'User Deleted',
        description: 'The user has been successfully deleted.',
      });
      fetchUsers();
    } catch (error) {
      console.error('Failed to delete user', error);
      notification.error({
        message: 'Deletion Failed',
        description: 'Failed to delete the user.',
      });
    }
  };

  return (
    <div style={{ padding: '24px' }}>
      <Space style={{ marginBottom: '16px' }}>
        <Input
          placeholder="Search for users..."
          prefix={<SearchOutlined />}
          onChange={(e) => handleFilterChange(e.target.value, 'search')}
        />
        <Select
          placeholder="Select Gender"
          onChange={(value) => handleFilterChange(value, 'gender')}
          style={{ width: 150 }}
        >
          <Option value="">All Genders</Option>
          <Option value="Male">Male</Option>
          <Option value="Female">Female</Option>
        </Select>
        <Select
          placeholder="Select Age Group"
          onChange={(value) => handleFilterChange(value, 'ageGroup')}
          style={{ width: 150 }}
        >
          <Option value="">All Age Groups</Option>
          <Option value="Under 10">Under 10</Option>
          <Option value="10-15">10-15</Option>
          <Option value="15-18">15-18</Option>
        </Select>
        <Select
          placeholder="Select Status"
          onChange={(value) => handleFilterChange(value, 'status')}
          style={{ width: 150 }}
        >
          <Option value="">All Statuses</Option>
          <Option value="Active">Active</Option>
          <Option value="Inactive">Inactive</Option>
        </Select>
        <Button type="danger" onClick={handleBulkDelete} disabled={!selectedUsers.length}>
          Delete Selected
        </Button>
        <Button icon={<ExportOutlined />} onClick={exportToPDF}>
          Export to PDF
        </Button>
        <Button icon={<FileExcelOutlined />} onClick={exportToExcel}>
          Export to Excel
        </Button>
      </Space>
      <Table
        rowSelection={{
          type: 'checkbox',
          ...rowSelection,
        }}
        columns={columns}
        dataSource={users}
        loading={loading}
        rowKey="id"
        id="userTable"
      />
    </div>
  );
};

export default UserList;
