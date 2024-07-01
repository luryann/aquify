import React, { useEffect, useState } from 'react';
import { Table, Input, Select, Button, Space, Card, notification } from 'antd';
import { SearchOutlined, FileExcelOutlined, ExportOutlined } from '@ant-design/icons';
import axios from 'axios';
import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';

const { Option } = Select;

const MeetResults: React.FC = () => {
  const [results, setResults] = useState([]);
  const [loading, setLoading] = useState(false);
  const [filters, setFilters] = useState({
    search: '',
    meet: '',
    swimmer: '',
    event: '',
  });

  useEffect(() => {
    fetchResults();
  }, [filters]);

  const fetchResults = async () => {
    setLoading(true);
    try {
      const response = await axios.get('/api/meet-results', { params: filters });
      setResults(response.data);
    } catch (error) {
      console.error('Failed to fetch meet results', error);
      notification.error({
        message: 'Fetch Failed',
        description: 'Failed to fetch meet results.',
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
    autoTable(doc, { html: '#meetResultsTable' });
    doc.save('meet-results.pdf');
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
      title: 'Meet Name',
      dataIndex: 'meetName',
      key: 'meetName',
    },
    {
      title: 'Swimmer',
      dataIndex: 'swimmer',
      key: 'swimmer',
    },
    {
      title: 'Event',
      dataIndex: 'event',
      key: 'event',
    },
    {
      title: 'Time',
      dataIndex: 'time',
      key: 'time',
    },
    {
      title: 'Date',
      dataIndex: 'date',
      key: 'date',
    },
    {
      title: 'Statistics',
      key: 'statistics',
      render: (text: any, record: any) => (
        <Space size="middle">
          <Button onClick={() => viewStatistics(record)}>View Statistics</Button>
        </Space>
      ),
    },
  ];

  const viewStatistics = (result: any) => {
    Modal.info({
      title: 'Swimmer Performance Statistics',
      content: (
        <div>
          <p><strong>Swimmer:</strong> {result.swimmer}</p>
          <p><strong>Event:</strong> {result.event}</p>
          <p><strong>Time:</strong> {result.time}</p>
          <p><strong>Meet:</strong> {result.meetName}</p>
          <p><strong>Date:</strong> {result.date}</p>
          <p><strong>Analysis:</strong></p>
          <ul>
            {result.statistics.map((stat: any, index: number) => (
              <li key={index}>{stat}</li>
            ))}
          </ul>
          <p><strong>Comparison:</strong></p>
          <ul>
            {result.comparison.map((comp: any, index: number) => (
              <li key={index}>{comp}</li>
            ))}
          </ul>
        </div>
      ),
      onOk() {},
    });
  };

  return (
    <div style={{ padding: '24px' }}>
      <Card title="Meet Results">
        <Space style={{ marginBottom: '16px' }}>
          <Input
            placeholder="Search for results..."
            prefix={<SearchOutlined />}
            onChange={(e) => handleFilterChange(e.target.value, 'search')}
          />
          <Select
            placeholder="Select Meet"
            onChange={(value) => handleFilterChange(value, 'meet')}
            style={{ width: 150 }}
          >
            {/* Mock meet options, replace with actual data */}
            <Option value="">All Meets</Option>
            <Option value="Meet1">Meet 1</Option>
            <Option value="Meet2">Meet 2</Option>
          </Select>
          <Select
            placeholder="Select Swimmer"
            onChange={(value) => handleFilterChange(value, 'swimmer')}
            style={{ width: 150 }}
          >
            {/* Mock swimmer options, replace with actual data */}
            <Option value="">All Swimmers</Option>
            <Option value="Swimmer1">Swimmer 1</Option>
            <Option value="Swimmer2">Swimmer 2</Option>
          </Select>
          <Select
            placeholder="Select Event"
            onChange={(value) => handleFilterChange(value, 'event')}
            style={{ width: 150 }}
          >
            {/* Mock event options, replace with actual data */}
            <Option value="">All Events</Option>
            <Option value="Event1">Event 1</Option>
            <Option value="Event2">Event 2</Option>
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
          dataSource={results}
          loading={loading}
          rowKey="id"
          pagination={{ pageSize: 10 }}
          id="meetResultsTable"
        />
      </Card>
    </div>
  );
};

export default MeetResults;
