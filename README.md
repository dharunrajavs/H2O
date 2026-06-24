Act as a Principal System Architect, Senior Go Backend Engineer, GPS Tracking Domain Expert, TCP Networking Expert, and High-Performance Distributed Systems Engineer.

I am building a GPS Tracking Platform from scratch.

Business Context:
- Our company sells GPS tracking devices.
- Each GPS device contains an Airtel SIM card.
- Devices communicate using GPT06 protocol over TCP.
- Every GPS device has a unique IMEI number.
- Thousands of GPS devices will continuously send location packets to our platform.
- Customers use Android/iOS mobile applications to view real-time vehicle tracking.

Core Requirement:
When a GPS device is installed in a vehicle:

1. GPS Device captures:
   - Latitude
   - Longitude
   - Speed
   - Direction
   - Ignition Status
   - Battery Status
   - Network Information
   - Timestamp
   - Device Health Data

2. Device sends binary packets using GPT06 protocol over TCP.

3. Backend must:
   - Accept TCP socket connections.
   - Identify device using IMEI.
   - Parse and decode GPT06 binary packets.
   - Validate packet structure.
   - Handle heartbeat packets.
   - Handle login packets.
   - Handle GPS packets.
   - Handle alarm packets.
   - Handle status packets.
   - Send acknowledgements back to device.
   - Store decoded data efficiently.

4. Real-time data should be pushed to:
   - Mobile App
   - Web Dashboard
   - Fleet Management Portal

5. Vehicle location updates must appear with minimum latency.

Technical Requirements:
- Backend: Golang
- Protocol: GPT06
- Transport: TCP
- Database: PostgreSQL
- Cache: Redis
- Realtime Engine: WebSocket
- Architecture: High Performance Event Driven Architecture
- Deployment: Docker + Kubernetes Ready
- Multi-Tenant Support
- Horizontal Scalability

Design Goals:
- Support 100,000+ concurrent GPS devices.
- Support millions of packets per day.
- Latency below 1 second for live tracking.
- Fault Tolerance.
- Auto Reconnect Handling.
- Packet Retry Handling.
- Duplicate Packet Prevention.
- High Availability.
- Efficient Memory Usage.
- Optimized CPU Usage.

Generate:

1. Complete System Architecture
2. Component Diagram
3. TCP Gateway Design
4. GPT06 Packet Flow
5. Binary Decoder Design
6. IMEI Registration Flow
7. Real-Time Tracking Flow
8. Database Schema
9. Redis Caching Strategy
10. WebSocket Architecture
11. Event Processing Architecture
12. Scaling Strategy for 100K Devices
13. Security Architecture
14. Monitoring & Logging Architecture
15. Deployment Architecture
16. API Design
17. Mobile App Data Flow
18. Sequence Diagrams
19. Failure Recovery Strategy
20. Performance Optimization Techniques

Output should be production-grade and suitable for enterprise-level GPS tracking software similar to Wialon, Traccar, or Fleet Management Systems.
Create both frontend and backend
