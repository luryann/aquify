import React, { useEffect, useState } from 'react';
import { List, Button } from 'antd';
import { useAppContext } from '../../contexts/AppContext';
import api from '../../utils/api';

const EventOverview: React.FC = () => {
  const { addNotification } = useAppContext();
  const [events, setEvents] = useState([]);

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

  const handleModifyEvent = async (eventId) => {
    try {
      await api.put(`/admin/events/${eventId}/modify`);
      addNotification({ id: Date.now(), message: 'Event modified successfully' });
    } catch (error) {
      addNotification({ id: Date.now(), message: 'Failed to modify event' });
    }
  };

  return (
    <div style={{ padding: '24px' }}>
      <List
        dataSource={events}
        renderItem={(event) => (
          <List.Item actions={[<Button onClick={() => handleModifyEvent(event.id)}>Modify</Button>]}>
            <List.Item.Meta
              title={event.name}
              description={`Date: ${event.date}`}
            />
          </List.Item>
        )}
      />
    </div>
  );
};

export default EventOverview;
