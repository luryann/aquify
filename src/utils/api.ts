import axios from 'axios';

const api = axios.create({
  baseURL: 'https://your-backend-api.com', // Replace with your actual backend URL
  headers: {
    'Content-Type': 'application/json',
  },
});

export default api;
