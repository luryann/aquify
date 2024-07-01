import React, { useEffect, useState } from 'react';
import { Table, Input, Select, Button, Space, Tag, notification } from 'antd';
import { SearchOutlined, FileExcelOutlined, ExportOutlined } from '@ant-design/icons';
import axios from 'axios';
import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';

const { Option } = Select;

const AuditLog: React.FC = () => {
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [filters, setFilters] = useState({
    search: '',
    user: '',
    action: '',
    dateRange: '',
  });

  useEffect(() => {
    fetchLogs();
  }, [filters]);

  const fetchLogs = async () => {
    setLoading(true);
    try {
      const response = await axios.get('/api/audit-logs', { params: filters });
      setLogs(response.data);
    } catch (error) {
      console.error('Failed to fetch audit logs', error);
      notification.error({
        message: 'Fetch Failed',
        description: 'Failed to fetch audit logs.',
      });
    } finally {
      setLoading(false);
    }
  };

  const handleFilterChange = (value: any, key: string) => {
    setFilters({ ...filters, [key]: value });
  };

  const exportToPDF = () => {
    const doc = new jsPDF();
    autoTable(doc, { html: '#auditLogTable' });
    doc.save('audit-logs.pdf');
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
      title: 'Timestamp',
      dataIndex: 'timestamp',
      key: 'timestamp',
    },
    {
      title: 'User',
      dataIndex: 'user',
      key: 'user',
    },
    {
      title: 'Action',
      dataIndex: 'action',
      key: 'action',
    },
    {
      title: 'Details',
      dataIndex: 'details',
      key: 'details',
      render: (details: string) => <Tag>{details}</Tag>,
    },
  ];

  return (
    <div style={{ padding: '24px' }}>
      <Space style={{ marginBottom: '16px' }}>
        <Input
          placeholder="Search logs..."
          prefix={<SearchOutlined />}
          onChange={(e) => handleFilterChange(e.target.value, 'search')}
        />
        <Select
          placeholder="Select User"
          onChange={(value) => handleFilterChange(value, 'user')}
          style={{ width: 150 }}
        >
          {/* Mock user options, replace with actual data */}
          <Option value="">All Users</Option>
          <Option value="User1">User1</Option>
          <Option value="User2">User2</Option>
        </Select>
        <Select
          placeholder="Select Action"
          onChange={(value) => handleFilterChange(value, 'action')}
          style={{ width: 150 }}
        >
          {/* Mock action options, replace with actual data */}
          <Option value="">All Actions</Option>
          <Option value="Create">Create</Option>
          <Option value="Update">Update</Option>
          <Option value="Delete">Delete</Option>
        </Select>
        <Select
          placeholder="Select Date Range"
          onChange={(value) => handleFilterChange(value, 'dateRange')}
          style={{ width: 150 }}
        >
          {/* Mock date range options, replace with actual data */}
          <Option value="">All Dates</Option>
          <Option value="Last 7 Days">Last 7 Days</Option>
          <Option value="Last 30 Days">Last 30 Days</Option>
          <Option value="Last 60 Days">Last 60 Days</Option>
        </Select>
        <Button icon={<ExportOutlined />} onClick={exportToPDF}>
          Export to PDF
        </Button>
        <Button icon={<FileExcelOutlined />} onClick={exportToExcel}>
          Export to Excel
        </Button>
      </Space>
      <Table
        columns={columns}
        dataSource={logs}
        loading={loading}
        rowKey="id"
        pagination={{ pageSize: 10 }}
        id="auditLogTable"
      />
    </div>
  );
};

export default AuditLog;
