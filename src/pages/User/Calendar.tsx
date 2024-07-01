import React, { useEffect, useState } from 'react';
import { Calendar, Card, message } from 'antd';
import api from '../../utils/api';

const CalendarPage: React.FC = () => {
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    fetchEvents();
  }, []);

  const fetchEvents = async () => {
    setLoading(true);
    try {
      const response = await api.get('/events');
      setEvents(response.data);
    } catch (error) {
      message.error('Failed to fetch events');
    } finally {
      setLoading(false);
    }
  };

  const dateCellRender = (value: any) => {
    const dayEvents = events.filter((event: any) => event.date === value.format('YYYY-MM-DD'));
    return (
      <ul>
        {dayEvents.map((event: any) => (
          <li key={event.id}>{event.name}</li>
        ))}
      </ul>
    );
  };

  return (
    <div style={{ padding: '24px' }}>
      <Card title="Team Functions Calendar">
        <Calendar dateCellRender={dateCellRender} />
      </Card>
    </div>
  );
};

export default CalendarPage;
