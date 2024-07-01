import React, { useState } from 'react';
import { Layout, Menu } from 'antd';
import { Link, Outlet } from 'react-router-dom';
import Header from '../../header';
import {
  HomeOutlined,
  UserOutlined,
  TeamOutlined,
  CalendarOutlined,
  NotificationOutlined,
  TrophyOutlined,
  MailOutlined,
  FileTextOutlined
} from '@ant-design/icons';

const { Sider, Content } = Layout;

const items = [
  { key: '1', icon: <HomeOutlined />, label: <Link to="/home">Home</Link> },
  { key: '2', icon: <UserOutlined />, label: <Link to="/my-account">My Account</Link> },
  { key: '3', icon: <TeamOutlined />, label: <Link to="/swim-groups">Swim Groups</Link> },
  { key: '4', icon: <CalendarOutlined />, label: <Link to="/calendar">Calendar</Link> },
  { key: '5', icon: <NotificationOutlined />, label: <Link to="/announcements">Announcements</Link> },
  { key: '6', icon: <TrophyOutlined />, label: <Link to="/events">Events & Competition</Link> },
  { key: '7', icon: <FileTextOutlined />, label: <Link to="/meet-results">Meet Results</Link> },
  { key: '8', icon: <MailOutlined />, label: <Link to="/contact">Contact</Link> },
];

const UserSidebar: React.FC = () => {
  const [collapsed, setCollapsed] = useState(false);

  const toggleSidebar = () => {
    setCollapsed(!collapsed);
  };

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header onToggleSidebar={toggleSidebar} />
      <Layout>
        <Sider collapsible collapsed={collapsed} width={250} className="site-layout-background" trigger={null}>
          <Menu
            theme="light"
            mode="inline"
            defaultSelectedKeys={['1']}
            items={items}
            style={{ height: '100%', borderRight: 0 }}
          />
        </Sider>
        <Layout style={{ padding: '0 24px 24px' }}>
          <Content
            className="site-layout-background"
            style={{
              padding: 24,
              margin: 0,
              minHeight: 280,
            }}
          >
            <Outlet />
          </Content>
        </Layout>
      </Layout>
    </Layout>
  );
};

export default UserSidebar;
