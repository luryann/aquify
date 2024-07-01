import React, { useEffect, useState } from 'react';
import { Table, Input, Button, Select, Space, Modal, Form, notification } from 'antd';
import { SearchOutlined, ExportOutlined, FileExcelOutlined } from '@ant-design/icons';
import axios from 'axios';
import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';

const { Option } = Select;

const Events: React.FC = () => {
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(false);
  const [filters, setFilters] = useState({
    search: '',
    status: '',
  });
  const [rsvpModalVisible, setRsvpModalVisible] = useState(false);
  const [selectedEvent, setSelectedEvent] = useState(null);

  useEffect(() => {
    fetchEvents();
  }, [filters]);

  const fetchEvents = async () => {
    setLoading(true);
    try {
      const response = await axios.get('/api/events', { params: filters });
      setEvents(response.data);
    } catch (error) {
      console.error('Failed to fetch events', error);
      notification.error({
        message: 'Fetch Failed',
        description: 'Failed to fetch events.',
      });
    } finally {
      setLoading(false);
    }
  };

  const handleFilterChange = (value: any, key: string) => {
    setFilters({ ...filters, [key]: value });
  };

  const handleRsvp = (event: any) => {
    setSelectedEvent(event);
    setRsvpModalVisible(true);
  };

  const handleRsvpSubmit = async (values: any) => {
    try {
      await axios.post(`/api/events/${selectedEvent.id}/rsvp`, values);
      notification.success({
        message: 'RSVP Successful',
        description: 'Your RSVP has been submitted.',
      });
      setRsvpModalVisible(false);
      fetchEvents();
    } catch (error) {
      console.error('Failed to submit RSVP', error);
      notification.error({
        message: 'RSVP Failed',
        description: 'Failed to submit your RSVP.',
      });
    }
  };

  const exportToPDF = () => {
    const doc = new jsPDF();
    autoTable(doc, { html: '#eventsTable' });
    doc.save('events.pdf');
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
      title: 'Event Name',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: 'Date',
      dataIndex: 'date',
      key: 'date',
    },
    {
      title: 'Location',
      dataIndex: 'location',
      key: 'location',
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <span style={{ color: status === 'Upcoming' ? 'green' : 'red' }}>
          {status}
        </span>
      ),
    },
    {
      title: 'Actions',
      key: 'actions',
      render: (text: any, record: any) => (
        <Button type="primary" onClick={() => handleRsvp(record)}>
          RSVP
        </Button>
      ),
    },
  ];

  return (
    <div style={{ padding: '24px' }}>
      <Space style={{ marginBottom: '16px' }}>
        <Input
          placeholder="Search for events..."
          prefix={<SearchOutlined />}
          onChange={(e) => handleFilterChange(e.target.value, 'search')}
        />
        <Select
          placeholder="Select Status"
          onChange={(value) => handleFilterChange(value, 'status')}
          style={{ width: 150 }}
        >
          <Option value="">All Statuses</Option>
          <Option value="Upcoming">Upcoming</Option>
          <Option value="Past">Past</Option>
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
        dataSource={events}
        loading={loading}
        rowKey="id"
        pagination={{ pageSize: 10 }}
        id="eventsTable"
      />
      <Modal
        title="RSVP for Event"
        visible={rsvpModalVisible}
        onCancel={() => setRsvpModalVisible(false)}
        footer={null}
      >
        <Form layout="vertical" onFinish={handleRsvpSubmit}>
          <Form.Item label="Attending" name="attending" rules={[{ required: true, message: 'Please select your attendance status!' }]}>
            <Select placeholder="Select Attendance">
              <Option value="yes">Yes</Option>
              <Option value="no">No</Option>
            </Select>
          </Form.Item>
          <Form.Item label="Notes" name="notes">
            <Input.TextArea rows={4} placeholder="Add any notes for the event organizers..." />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit">
              Submit RSVP
            </Button>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default Events;
