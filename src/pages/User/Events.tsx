import React, { useEffect, useState } from 'react';
import { Tabs, Input, Button, Select, Space, Modal, Form, notification, Checkbox } from 'antd';
import { SearchOutlined, ExportOutlined, FileExcelOutlined } from '@ant-design/icons';
import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';
import axios from 'axios';

const { Option } = Select;
const { TabPane } = Tabs;

const Events: React.FC = () => {
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(false);
  const [filters, setFilters] = useState({ search: '', status: '' });
  const [rsvpModalVisible, setRsvpModalVisible] = useState(false);
  const [selectedEvent, setSelectedEvent] = useState(null);
  const [selectedChild, setSelectedChild] = useState(null);
  const [eventDetailsModalVisible, setEventDetailsModalVisible] = useState(false);
  const [attending, setAttending] = useState('');
  const [isAdmin, setIsAdmin] = useState(false); // Mock admin state
  const [meetEvents, setMeetEvents] = useState([]);
  const [submittedEvents, setSubmittedEvents] = useState([]); // Mock submitted events
  const [children, setChildren] = useState([
    { id: 1, name: 'Ryan Lu' },
    { id: 2, name: 'Jane Doe' },
  ]);

  useEffect(() => {
    // Uncomment and modify this function when the API is available
    /*
    fetchEvents();
    */
    // Check if the user is admin
    setIsAdmin(true); // Replace with actual logic to determine if user is admin

    // Mock data
    setEvents([
      { id: 1, name: 'POOL CLOSURE - Independence Day', date: '2024-07-04', location: 'Main Pool', status: 'Upcoming', description: '07/04 Independence Day' },
      { id: 2, name: '2024 Los Angeles Invitational', date: '2024-07-05', location: 'Los Angeles', status: 'Upcoming', description: 'Qualified Sr. Swimmers ONLY' },
      { id: 3, name: 'Metro Summer Festival (1day)', date: '2024-07-07', location: 'Metro', status: 'Upcoming', description: 'Age Group' },
      { id: 4, name: 'Summer Metro Champs - LKWD (MANDATORY)', date: '2024-07-13', location: 'LKWD', status: 'Upcoming', description: 'Qualified Sr. Swimmers ONLY' },
      { id: 5, name: 'SCS - Age Group Elite Meet (14&UNDER ONLY)', date: '2024-07-18', location: 'Mission Viejo, CA', status: 'Upcoming', description: 'Qualifying Times ONLY' },
      { id: 6, name: 'Past Event 1', date: '2024-06-10', location: 'Past Location 1', status: 'Past', description: 'Past Event Description 1' },
      { id: 7, name: 'Past Event 2', date: '2024-06-15', location: 'Past Location 2', status: 'Past', description: 'Past Event Description 2' },
    ]);

    // Mock submitted events data
    setSubmittedEvents([
      { id: 1, eventName: '100 Fly', entryTime: '1:07.72Y', approval: true },
      { id: 2, eventName: '100 Back', entryTime: '1:07.93Y', approval: false },
      { id: 3, eventName: '100 Breast', entryTime: '1:10.41Y', approval: false },
    ]);
  }, [filters]);

  /*
  const fetchEvents = async () => {
    setLoading(true);
    try {
      const response = await axios.get('/api/events', { params: filters });
      setEvents(response.data);
    } catch (error) {
      console.error('Failed to fetch events', error);
      notification.error({ message: 'Fetch Failed', description: 'Failed to fetch events.' });
    } finally {
      setLoading(false);
    }
  };
  */

  const handleFilterChange = (value: any, key: string) => {
    setFilters({ ...filters, [key]: value });
  };

  const handleRsvp = (event: any) => {
    setSelectedEvent(event);
    setRsvpModalVisible(true);
  };

  const handleRsvpSubmit = async (values: any) => {
    try {
      // Uncomment and modify this function when the API is available
      // await axios.post(`/api/events/${selectedEvent.id}/rsvp`, values);
      notification.success({ message: 'RSVP Successful', description: 'Your RSVP has been submitted.' });
      setRsvpModalVisible(false);
      if (values.attending === 'yes') {
        // Fetch meet events for the selected child
        // Uncomment and modify this function when the API is available
        // const response = await axios.get(`/api/meet-events?childId=${selectedChild}`);
        // setMeetEvents(response.data);
        setMeetEvents([
          { id: 1, name: '100 Fly', bestTime: '1:07.72Y', entryTime: '1:07.72Y', qualified: true },
          { id: 2, name: '100 Back', bestTime: '1:07.93Y', entryTime: '1:07.93Y', qualified: true },
          { id: 3, name: '100 Breast', bestTime: '1:10.41Y', entryTime: '1:10.41Y', qualified: false },
          { id: 4, name: '100 Free', bestTime: '56.69Y', entryTime: '56.69Y', qualified: false },
          { id: 5, name: '200 Medley', bestTime: '2:21.98Y', entryTime: '2:21.98Y', qualified: false },
        ]);
        setEventDetailsModalVisible(true);
      }
      // fetchEvents();
    } catch (error) {
      console.error('Failed to submit RSVP', error);
      notification.error({ message: 'RSVP Failed', description: 'Failed to submit your RSVP.' });
    }
  };

  const exportToPDF = () => {
    const doc = new jsPDF();
    autoTable(doc, { html: '#eventsTable' });
    doc.save('events.pdf');
  };

  const exportToExcel = () => {
    notification.info({ message: 'Export to Excel', description: 'Export to Excel functionality is not implemented yet.' });
  };

  const renderEvents = (eventsList: any) => {
    return eventsList.map((event: any) => (
      <div key={event.id} style={{ marginBottom: '16px', borderBottom: '1px solid #f0f0f0', paddingBottom: '16px' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div onClick={() => handleEventClick(event)}>
            <div style={{ fontSize: '16px', fontWeight: 'bold', color: 'black' }}>{event.name}</div>
            <div>{event.date}</div>
            <div>{event.location}</div>
            <div>{event.description}</div>
          </div>
          <Button type="primary" onClick={() => handleRsvp(event)}>Attend / Decline</Button>
        </div>
      </div>
    ));
  };

  return (
    <div style={{ padding: '24px' }}>
      <Space style={{ marginBottom: '16px', display: 'flex', justifyContent: 'space-between' }}>
        <Input
          placeholder="Search for events..."
          prefix={<SearchOutlined />}
          onChange={(e) => handleFilterChange(e.target.value, 'search')}
          style={{ width: '300px' }}
        />
        <Space>
          <Button icon={<ExportOutlined />} onClick={exportToPDF}>
            Export to PDF
          </Button>
          <Button icon={<FileExcelOutlined />} onClick={exportToExcel}>
            Export to Excel
          </Button>
        </Space>
      </Space>
      <Tabs defaultActiveKey="1">
        <TabPane tab="Current & Upcoming" key="1">
          {renderEvents(events.filter((event: any) => event.status === 'Upcoming'))}
        </TabPane>
        <TabPane tab="Past & Archived" key="2">
          {renderEvents(events.filter((event: any) => event.status === 'Past'))}
        </TabPane>
      </Tabs>

      <Modal
        title="RSVP for Event"
        visible={rsvpModalVisible}
        onCancel={() => setRsvpModalVisible(false)}
        footer={null}
      >
        <Form layout="vertical" onFinish={handleRsvpSubmit}>
          <Form.Item label="Select Child" name="child" rules={[{ required: true, message: 'Please select a child!' }]}>
            <Select placeholder="Select Child" onChange={(value) => setSelectedChild(value)}>
              {children.map((child) => (
                <Option key={child.id} value={child.id}>{child.name}</Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item label="Attending" name="attending" rules={[{ required: true, message: 'Please select your attendance status!' }]}>
            <Select placeholder="Select Attendance" onChange={(value) => setAttending(value)}>
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

      <Modal
        title={selectedEvent?.name}
        visible={eventDetailsModalVisible}
        onCancel={() => setEventDetailsModalVisible(false)}
        footer={<Button type="primary" onClick={() => setEventDetailsModalVisible(false)}>Commit Changes</Button>}
        width={1000}
      >
        {selectedEvent && (
          <div>
            <div><b>Meet Name:</b> {selectedEvent.name}</div>
            <div><b>Location:</b> {selectedEvent.location}</div>
            <div><b>Start Date:</b> {selectedEvent.startDate}</div>
            <div><b>End Date:</b> {selectedEvent.endDate}</div>
            <div><b>Age Group:</b> {selectedEvent.ageGroup}</div>
            <div><b>Details:</b> {selectedEvent.details}</div>
            <div><b>Committed Sessions:</b> None</div>
            {selectedEvent.registrationDeadline < new Date().toISOString() && (
              <div style={{ color: 'red', marginBottom: '16px' }}>Registration Deadline has passed.</div>
            )}
            <div style={{ marginTop: '16px' }}>
              <b>Important Notes:</b>
              <ol>
                <li>Please pick the individual events below that the Athlete wants to attend.</li>
                <li>Relay teams are solely determined by the coaches. If you have a problem attending the relay teams, please contact the coaches directly.</li>
                <li>You cannot make changes after a coach has approved or rejected your events. Please contact a coach to request any changes.</li>
                <li>Only an Admin can customize [Entry Times] and set [Bonus] and [Exhibition] fields.</li>
              </ol>
            </div>
            <div style={{ marginTop: '16px' }}>
              <b>Day 1 Session 1</b>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
                <div style={{ width: '10%' }}>Best Time</div>
                <div style={{ width: '10%' }}>Entry Time</div>
                <div style={{ width: '10%' }}>Bonus</div>
                <div style={{ width: '10%' }}>Exhibition</div>
                <div style={{ width: '10%' }}>Approval</div>
                <div style={{ width: '10%' }}>Ev#</div>
                <div style={{ width: '10%' }}>Competitive Category</div>
                <div style={{ width: '10%' }}>Event</div>
                <div style={{ width: '10%' }}>Qualify Time</div>
              </div>
              {meetEvents.map((event) => (
                <div key={event.id} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
                  <Checkbox />
                  <div style={{ width: '10%' }}>{event.bestTime}</div>
                  <Input style={{ width: '10%' }} defaultValue={event.entryTime} disabled={!isAdmin} />
                  <Checkbox style={{ width: '10%' }} disabled={!isAdmin} />
                  <Checkbox style={{ width: '10%' }} disabled={!isAdmin} />
                  <Checkbox style={{ width: '10%' }} disabled={!isAdmin} />
                  <div style={{ width: '10%' }}>{event.id}D</div>
                  <div style={{ width: '10%' }}>M</div>
                  <div style={{ width: '10%' }}>{event.name}</div>
                  <div style={{ width: '10%', color: event.qualified ? 'green' : 'red' }}>{event.qualified ? 'Qualified' : 'Not Qualified'}</div>
                </div>
              ))}
            </div>
            {submittedEvents.length > 0 && (
              <div style={{ marginTop: '16px' }}>
                <b>Submitted Events:</b>
                {submittedEvents.map((event) => (
                  <div key={event.id} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
                    <div style={{ width: '10%' }}>{event.eventName}</div>
                    <div style={{ width: '10%' }}>{event.entryTime}</div>
                    <div style={{ width: '10%', color: event.approval ? 'green' : 'orange' }}>{event.approval ? 'Approved' : 'Pending'}</div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </Modal>
    </div>
  );
};

export default Events;
