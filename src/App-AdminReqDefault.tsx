import React from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import { Refine } from '@refinedev/core';
import { ThemedLayoutV2, RefineThemes, notificationProvider } from '@refinedev/antd';
import { ConfigProvider } from 'antd';
import { useAuth0 } from '@auth0/auth0-react';
import UserSidebar from './components/Sidebar/UserSidebar';
import AdminSidebar from './components/Sidebar/AdminSidebar';
import Home from './pages/User/Home';
import MyAccount from './pages/User/MyAccount';
import SwimGroups from './pages/User/SwimGroups';
import Calendar from './pages/User/Calendar';
import Announcements from './pages/User/Announcements';
import Events from './pages/User/Events';
import MeetResults from './pages/User/MeetResults';
import Contact from './pages/User/Contact';
import EventOverview from './pages/Admin/EventOverview';
import AnnouncementCreator from './pages/Admin/AnnouncementCreator';
import MassEmail from './pages/Admin/MassEmail';
import EventCreator from './pages/Admin/EventCreator';
import UserList from './pages/Admin/UserList';
import AuditLog from './pages/Admin/AuditLog';
import Login from './pages/Login';
import NotFound from './pages/NotFound';
import NotAuthorized from './pages/NotAuthorized';

const App: React.FC = () => {
  const { isAuthenticated, isLoading, user } = useAuth0();

  if (isLoading) {
    return <div>Loading...</div>;
  }

  const isAdmin = isAuthenticated && user && user['http://example.com/roles'] && user['http://example.com/roles'].includes('admin');

  return (
    <ConfigProvider theme={RefineThemes.Orange}>
      <Refine
        notificationProvider={notificationProvider}
        Layout={ThemedLayoutV2}
        Title={() => <div>Aquify</div>}
        Sider={() => isAuthenticated ? (isAdmin ? <AdminSidebar /> : <UserSidebar />) : null}
      >
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/" element={isAuthenticated ? <UserSidebar /> : <Navigate to="/login" />}>
            <Route path="home" element={<Home />} />
            <Route path="my-account" element={<MyAccount />} />
            <Route path="swim-groups" element={<SwimGroups />} />
            <Route path="calendar" element={<Calendar />} />
            <Route path="announcements" element={<Announcements />} />
            <Route path="events" element={<Events />} />
            <Route path="meet-results" element={<MeetResults />} />
            <Route path="contact" element={<Contact />} />
            <Route path="*" element={<NotFound />} /> {/* Catch-all route for 404 */}
          </Route>
          <Route
            path="/admin/*"
            element={isAdmin ? <AdminSidebar /> : <Navigate to="/not-authorized" />}
          >
            <Route path="event-overview" element={<EventOverview />} />
            <Route path="announcement-creator" element={<AnnouncementCreator />} />
            <Route path="mass-email" element={<MassEmail />} />
            <Route path="event-creator" element={<EventCreator />} />
            <Route path="user-list" element={<UserList />} />
            <Route path="audit-log" element={<AuditLog />} />
            <Route path="*" element={<NotFound />} /> {/* Catch-all route for 404 */}
          </Route>
          <Route path="/not-authorized" element={<NotAuthorized />} />
        </Routes>
      </Refine>
    </ConfigProvider>
  );
};

export default App;
