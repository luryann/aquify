import React, { useEffect, useState } from 'react';
import { List, Button, Modal, Form, Input, notification, Row, Col, Typography, Checkbox } from 'antd';
import { useAppContext } from '../../contexts/AppContext';
import api from '../../utils/api';

const { Title, Text } = Typography;

const EventOverview: React.FC = () => {
  const { addNotification } = useAppContext();
  const [events, setEvents] = useState([]);
  const [selectedEvent, setSelectedEvent] = useState(null);
  const [eventDetailsModalVisible, setEventDetailsModalVisible] = useState(false);
  const [isAdmin, setIsAdmin] = useState(true); // Mock admin state

  useEffect(() => {
    const fetchEvents = async () => {
      try {
        const response = await api.get('/admin/events');
        setEvents(response.data);
      } catch (error) {
        addNotification({ id: Date.now(), message: 'Failed to fetch events' });
      }
    };

    fetchEvents();
  }, [addNotification]);

  const handleModifyEvent = (event) => {
    setSelectedEvent(event);
    setEventDetailsModalVisible(true);
  };

  const handleApprovalChange = async (eventId, swimmerId, approved) => {
    try {
      await api.put(`/admin/events/${eventId}/swimmers/${swimmerId}/approve`, { approved });
      addNotification({ id: Date.now(), message: 'Event modified successfully' });
      fetchEvents();
    } catch (error) {
      addNotification({ id: Date.now(), message: 'Failed to modify event' });
    }
  };

  return (
    <div style={{ padding: '24px' }}>
      <Title level={2}>Event Overview</Title>
      <List
        dataSource={events}
        renderItem={(event) => (
          <List.Item actions={[<Button onClick={() => handleModifyEvent(event)}>Modify</Button>]}>
            <List.Item.Meta
              title={event.name}
              description={`Date: ${event.date}`}
            />
          </List.Item>
        )}
      />

      <Modal
        title={selectedEvent?.name}
        visible={eventDetailsModalVisible}
        onCancel={() => setEventDetailsModalVisible(false)}
        footer={null}
        width={1000}
      >
        {selectedEvent && (
          <div>
            <Row gutter={16}>
              <Col span={8}><b>Meet Name:</b> {selectedEvent.name}</Col>
              <Col span={8}><b>Location:</b> {selectedEvent.location}</Col>
              <Col span={8}><b>Start Date:</b> {selectedEvent.startDate}</Col>
              <Col span={8}><b>End Date:</b> {selectedEvent.endDate}</Col>
              <Col span={8}><b>Age Group:</b> {selectedEvent.ageGroup}</Col>
              <Col span={8}><b>Details:</b> {selectedEvent.details}</Col>
            </Row>
            <Divider />
            <Title level={4}>Swimmer Entries</Title>
            {selectedEvent.swimmers?.map((swimmer) => (
              <div key={swimmer.id} style={{ marginBottom: '16px' }}>
                <Text strong>{swimmer.name}</Text>
                <List
                  dataSource={swimmer.events}
                  renderItem={(event) => (
                    <List.Item
                      actions={[
                        <Checkbox
                          checked={event.approved}
                          onChange={(e) => handleApprovalChange(selectedEvent.id, swimmer.id, e.target.checked)}
                        >
                          {event.approved ? 'Approved' : 'Pending'}
                        </Checkbox>,
                      ]}
                    >
                      <List.Item.Meta
                        title={event.name}
                        description={`Entry Time: ${event.entryTime}`}
                      />
                    </List.Item>
                  )}
                />
              </div>
            ))}
          </div>
        )}
      </Modal>
    </div>
  );
};

export default EventOverview;
