# Licensed under GNU Public License v3.0

# hy3parser
# Parser for Hy-Tek Meet Manager meet files. Mainly used for .hy3 files.

# The HY3 file format is used by HyTek Meet Manager software, which is extensively used in organizing swimming competitions. This format encapsulates data pertaining to events, participants, and results in a text-based file that features distinct record types identifiable by specific prefixes. Below is a technical breakdown of the key components relevant to parsing and processing HY3 files within the application:

# File Structure
# HY3 files are composed of lines, each beginning with a two-character prefix that indicates the type of data contained in that line. The structure includes multiple sections for events, athletes, and results. Each section has a defined format with fixed-width fields, making it suitable for regex-based parsing.

# C1 - Event Entries:
# Fields include meet codes, event types, subtypes, strokes (using FINA stroke codes), distances, gender, age ranges, and event dates.
# Example: C10103000501M08081011/25/2023 could be deciphered as an event (code 0103) for a 100-meter male freestyle for ages 8-10 on November 25, 2023.

# D3 - Participant Entries:
# Includes unique entry and swimmer IDs, names padded to a fixed length, team codes, seed and final times in MM
# .hh format, and a status code indicating the result or participation status.
# Example: D3000123456789John Doe ABC Swim 01:00.5001:00.25A denotes participant John Doe, with an entry ID of 123456789, from ABC Swim team, seeded at 1 minute 0.50 seconds and finishing at 1 minute 0.25 seconds.

# Parsing Mechanics

# Regex Patterns:
# Given the fixed format of HY3 lines, regex is used to extract structured data. Each line type has an associated regex pattern that describes how to parse the line into its constituent fields.
# For C1 lines, a pattern like r'^C1(\d{2})(\d{4})(\d{4})(.*)(\d{1})(\d{1})(\d{1})(\d{1})(\d{2}\/\d{2}\/\d{4})$' might be used, which extracts meet code, event type, subtype, stroke, distance, gender, age range, and date, respectively.
# For D3 lines, the pattern r'^D3(\d{4})(\d{4})(.*)([A-Z]{2})(\d{2}:\d{2}.\d{2})(\d{2}:\d{2}.\d{2})([A-Z]+)$' captures the entry ID, swimmer ID, name, team, seed time, final time, and status.

# Thread-Safe Database Operations:
# After parsing, data is often handled in bulk operations, necessitating thread-safe transactions. Python’s ThreadPoolExecutor is employed to process parsed data in parallel without interfering with the Flask application’s main execution thread or database session conflicts.

from flask import Flask, request, jsonify
from werkzeug.utils import secure_filename
from celery import Celery
import os
import re
import logging
from flask_sqlalchemy import SQLAlchemy
import aiofiles
from concurrent.futures import ThreadPoolExecutor

# Configure Flask app
app = Flask(__name__)
app.config['UPLOAD_FOLDER'] = 'uploads/'
app.config['SQLALCHEMY_DATABASE_URI'] = 'sqlite:///site.db'
app.config['CELERY_BROKER_URL'] = 'redis://localhost:6379/0'
app.config['CELERY_RESULT_BACKEND'] = 'redis://localhost:6379/0'
app.config['SQLALCHEMY_TRACK_MODIFICATIONS'] = False

# Initialize Celery
celery = Celery(app.name, broker=app.config['CELERY_BROKER_URL'])

# Initialize SQLAlchemy
db = SQLAlchemy(app)

# Define models
class Event(db.Model):
    id = db.Column(db.Integer, primary_key=True)
    event_number = db.Column(db.Integer, nullable=False)
    event_name = db.Column(db.String(120), nullable=False)
    event_date = db.Column(db.String(50), nullable=True)

class Participant(db.Model):
    id = db.Column(db.Integer, primary_key=True)
    event_number = db.Column(db.Integer, nullable=False)
    swimmer_id = db.Column(db.String(120), nullable=False)
    name = db.Column(db.String(200), nullable=False)
    team = db.Column(db.String(100), nullable=True)
    seed_time = db.Column(db.String(50), nullable=True)
    final_time = db.Column(db.String(50), nullable=True)
    status = db.Column(db.String(20))

# Setup logging
logging.basicConfig(filename='hytek_parser.log', level=logging.ERROR, format='%(asctime)s:%(levelname)s:%(message)s')

# Parser class
class HyTekParser:
    def __init__(self, content):
        self.content = content

    def parse_section(self, pattern, section_name):
        try:
            matches = re.finditer(pattern, self.content, re.MULTILINE)
            return [match.groups() for match in matches]
        except Exception as e:
            logging.error(f"Failed to parse {section_name}: {str(e)}")
            return []  # Continue with other sections

# File upload route
@app.route('/upload', methods=['POST'])
def upload_file():
    if 'file' not in request.files:
        return "No file part", 400
    file = request.files['file']
    if file.filename == '':
        return "No selected file", 400
    if file:
        filename = secure_filename(file.filename)
        file_path = os.path.join(app.config['UPLOAD_FOLDER'], filename)
        file.save(file_path)
        parse_hytek_file.delay(file_path)
        return "File uploaded and parsing initiated", 202

# Celery task
@celery.task
def parse_hytek_file(file_path):
    asyncio.run(process_file(file_path))

async def process_file(file_path):
    async with aiofiles.open(file_path, 'r') as file:
        content = await file.read()
    process_content(content)

def process_content(content):
    parser = HyTekParser(content)
    with ThreadPoolExecutor(max_workers=2) as executor:
        results = list(executor.map(parser.parse_section, [
            (r'^C1(\d{2})(\d{4})(\d{4})(.*)(\d{1})(\d{1})(\d{1})(\d{1})(\d{2}\/\d{2}\/\d{4})', "Events"),
            (r'^D3(\d{4})(\d{4})(.*)([A-Z]{2})(\d{2}:\d{2}.\d{2})(\d{2}:\d{2}.\d{2})([A-Z]+)', "Participants")
        ]))
        events, participants = results
        store_data(events, participants)

def store_data(events, participants):
    try:
        for event in events:
            new_event = Event(event_number=int(event[1]), event_name=event[3], event_date=event[8])
            db.session.add(new_event)
        for participant in participants:
            new_participant = Participant(
                event_number=int(participant[0]), 
                swimmer_id=participant[1], 
                name=participant[2],
                team=participant[3],
                seed_time=participant[4],
                final_time=participant[5],
                status=participant[6])
            db.session.add(new_participant)
        db.session.commit()
    except Exception as e:
        logging.error(f"Failed to store data: {str(e)}")

if __name__ == '__main__':
    db.create_all()
    app.run(debug=True)
