import React, { useState } from 'react';
import { Layout, Menu } from 'antd';
import { Link, Outlet } from 'react-router-dom';
import Header from '../../header';
import {
  NotificationOutlined,
  SendOutlined,
  FormOutlined,
  UsergroupAddOutlined
} from '@ant-design/icons';

const { Sider, Content } = Layout;

const items = [
  { key: '1', icon: <NotificationOutlined />, label: <Link to="/admin/event-overview">Event Overview</Link> },
  { key: '2', icon: <NotificationOutlined />, label: <Link to="/admin/announcement-creator">Announcements</Link> },
  { key: '3', icon: <SendOutlined />, label: <Link to="/admin/mass-email">Mass Email</Link> },
  { key: '4', icon: <FormOutlined />, label: <Link to="/admin/event-creator">Event Creator</Link> },
  { key: '5', icon: <UsergroupAddOutlined />, label: <Link to="/admin/user-list">User List</Link> }
];

const AdminSidebar: React.FC = () => {
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

export default AdminSidebar;
