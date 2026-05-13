#!/usr/bin/env python3
"""
Симуляция людей в Санкт-Петербурге
Люди перемещаются между достопримечательностями города
"""

import random
import math
import time
import json
from dataclasses import dataclass, field
from typing import List, Optional, Dict, Any
from enum import Enum


class PersonState(Enum):
    WALKING = "walking"
    VISITING = "visiting"
    RESTING = "resting"


@dataclass
class Landmark:
    """Достопримечательность Санкт-Петербурга"""
    name: str
    latitude: float
    longitude: float


@dataclass
class Person:
    """Человек в симуляции"""
    id: str
    name: str
    current_landmark: Optional[Landmark]
    target_landmark: Optional[Landmark]
    latitude: float
    longitude: float
    speed: float  # градусов в секунду
    state: PersonState = PersonState.WALKING
    visit_start_time: float = 0.0
    visit_duration: float = 120.0  # секунд
    heart_rate: int = 72
    temperature: float = 36.6
    co2: int = 400
    
    def to_dict(self) -> Dict[str, Any]:
        return {
            "id": self.id,
            "name": self.name,
            "latitude": round(self.latitude, 6),
            "longitude": round(self.longitude, 6),
            "current_landmark": self.current_landmark.name if self.current_landmark else None,
            "target_landmark": self.target_landmark.name if self.target_landmark else None,
            "state": self.state.value,
            "speed": round(self.speed * 1000000, 2),  # м/с
            "heart_rate": self.heart_rate,
            "temperature": round(self.temperature, 1),
            "co2": self.co2
        }


# Основные достопримечательности Санкт-Петербурга
SPB_LANDMARKS = [
    Landmark("Дворцовая площадь", 59.9398, 30.3146),
    Landmark("Эрмитаж", 59.9417, 30.3127),
    Landmark("Исаакиевский собор", 59.9335, 30.3048),
    Landmark("Спас на Крови", 59.9402, 30.3269),
    Landmark("Казанский собор", 59.9345, 30.3224),
    Landmark("Петропавловская крепость", 59.9502, 30.3159),
    Landmark("Невский проспект (начало)", 59.9341, 30.3163),
    Landmark("Невский проспект (конец)", 59.9295, 30.3586),
    Landmark("Марсово поле", 59.9445, 30.3208),
    Landmark("Летний сад", 59.9466, 30.3291),
    Landmark("Стрелка Васильевского острова", 59.9429, 30.3067),
    Landmark("Медный всадник", 59.9365, 30.3058),
    Landmark("Новая Голландия", 59.9208, 30.2936),
    Landmark("Елагин остров", 59.9676, 30.2926),
]


class PeopleSimulation:
    """Симуляция людей в Санкт-Петербурге"""
    
    def __init__(self, num_people: int = 10):
        self.landmarks = SPB_LANDMARKS
        self.people: List[Person] = []
        self.is_running = False
        self.interval = 2.0  # секунды между обновлениями
        
        # Инициализация людей
        self._init_people(num_people)
    
    def _init_people(self, num_people: int):
        """Инициализирует людей в случайных точках города"""
        for i in range(num_people):
            person = self._create_person(i)
            self.people.append(person)
    
    def _create_person(self, index: int) -> Person:
        """Создаёт нового человека"""
        # Выбираем случайную начальную достопримечательность
        start_landmark = random.choice(self.landmarks)
        
        # Небольшое смещение от достопримечательности
        offset_lat = (random.random() - 0.5) * 0.002
        offset_lon = (random.random() - 0.5) * 0.002
        
        lat = start_landmark.latitude + offset_lat
        lon = start_landmark.longitude + offset_lon
        
        # Скорость ходьбы: 4-6 км/ч -> ~0.00001-0.000015 градусов в секунду
        speed = 0.00001 + random.random() * 0.000005
        
        return Person(
            id=f"person_{index + 1}",
            name=f"Турист {index + 1}",
            current_landmark=start_landmark,
            target_landmark=self._select_new_target(start_landmark),
            latitude=lat,
            longitude=lon,
            speed=speed,
            state=PersonState.WALKING,
            visit_duration=120 + random.randint(0, 180),  # 2-5 минут
        )
    
    def _select_new_target(self, current: Landmark) -> Landmark:
        """Выбирает новую цель для посещения"""
        while True:
            candidate = random.choice(self.landmarks)
            if candidate.name != current.name:
                return candidate
    
    @staticmethod
    def _calculate_distance(lat1: float, lon1: float, lat2: float, lon2: float) -> float:
        """Вычисляет расстояние между двумя точками (в градусах)"""
        d_lat = lat2 - lat1
        d_lon = lon2 - lon1
        return math.sqrt(d_lat * d_lat + d_lon * d_lon)
    
    def _update_person(self, person: Person, now: float):
        """Обновляет состояние одного человека"""
        if person.state == PersonState.WALKING:
            self._update_walking(person, now)
        elif person.state == PersonState.VISITING:
            self._update_visiting(person, now)
        elif person.state == PersonState.RESTING:
            self._update_resting(person, now)
        
        # Генерируем метрики в зависимости от состояния
        self._generate_metrics(person)
    
    def _update_walking(self, person: Person, now: float):
        """Обновляет состояние идущего человека"""
        if person.target_landmark is None:
            person.target_landmark = self._select_new_target(person.current_landmark)
            return
        
        # Вычисляем направление к цели
        d_lat = person.target_landmark.latitude - person.latitude
        d_lon = person.target_landmark.longitude - person.longitude
        distance = self._calculate_distance(
            person.latitude, person.longitude,
            person.target_landmark.latitude, person.target_landmark.longitude
        )
        
        # Если достигли цели (расстояние < 10 метров ~ 0.0001 градуса)
        if distance < 0.0001:
            person.latitude = person.target_landmark.latitude
            person.longitude = person.target_landmark.longitude
            person.current_landmark = person.target_landmark
            person.state = PersonState.VISITING
            person.visit_start_time = time.time()
            print(f"[{person.name}] прибыл к {person.current_landmark.name}")
            return
        
        # Нормализуем направление и двигаемся
        direction_lat = d_lat / distance
        direction_lon = d_lon / distance
        
        person.latitude += direction_lat * person.speed
        person.longitude += direction_lon * person.speed
    
    def _update_visiting(self, person: Person, now: float):
        """Обновляет состояние посещающего человека"""
        elapsed = now - person.visit_start_time
        
        # Если провели достаточно времени у достопримечательности
        if elapsed >= person.visit_duration:
            # С вероятностью 30% переходим в состояние отдыха
            if random.random() < 0.3:
                person.state = PersonState.RESTING
                person.visit_start_time = now
                print(f"[{person.name}] отдыхает возле {person.current_landmark.name}")
            else:
                # Выбираем новую цель и идём
                person.target_landmark = self._select_new_target(person.current_landmark)
                person.state = PersonState.WALKING
                print(f"[{person.name}] направляется к {person.target_landmark.name}")
    
    def _update_resting(self, person: Person, now: float):
        """Обновляет состояние отдыхающего человека"""
        elapsed = now - person.visit_start_time
        
        # Отдыхаем 1-3 минуты
        rest_duration = 60 + random.randint(0, 120)
        if elapsed >= rest_duration:
            person.target_landmark = self._select_new_target(person.current_landmark)
            person.state = PersonState.WALKING
            print(f"[{person.name}] закончил отдых, направляется к {person.target_landmark.name}")
    
    def _generate_metrics(self, person: Person):
        """Генерирует метрики для человека"""
        # Базовые показатели
        base_heart_rate = 72
        base_temp = 36.6
        base_co2 = 400
        
        # Корректировка в зависимости от состояния
        if person.state == PersonState.WALKING:
            base_heart_rate += 20 + random.randint(0, 15)  # При ходьбе пульс выше
            base_co2 += 50 + random.randint(0, 30)
        elif person.state == PersonState.VISITING:
            base_heart_rate += 5 + random.randint(0, 10)
        elif person.state == PersonState.RESTING:
            base_heart_rate -= 5 + random.randint(0, 5)
        
        # Добавляем небольшой шум
        person.heart_rate = base_heart_rate + random.randint(-2, 2)
        person.temperature = base_temp + (random.random() * 0.3 - 0.15)
        person.co2 = base_co2 + random.randint(-10, 10)
    
    def update(self):
        """Обновляет состояние всех людей"""
        now = time.time()
        for person in self.people:
            self._update_person(person, now)
    
    def get_people_status(self) -> List[Dict[str, Any]]:
        """Возвращает статус всех людей"""
        return [person.to_dict() for person in self.people]
    
    def get_landmarks(self) -> List[Dict[str, Any]]:
        """Возвращает список достопримечательностей"""
        return [
            {
                "name": landmark.name,
                "latitude": landmark.latitude,
                "longitude": landmark.longitude
            }
            for landmark in self.landmarks
        ]
    
    def add_person(self) -> Person:
        """Добавляет нового человека в симуляцию"""
        index = len(self.people)
        person = self._create_person(index)
        self.people.append(person)
        print(f"Добавлен {person.name}")
        return person
    
    def remove_person(self, person_id: str) -> bool:
        """Удаляет человека из симуляции"""
        for i, person in enumerate(self.people):
            if person.id == person_id:
                removed = self.people.pop(i)
                print(f"Удалён {removed.name}")
                return True
        return False
    
    def teleport_person(self, person_id: str, landmark_name: str) -> bool:
        """Телепортирует человека к указанной достопримечательности"""
        target_landmark = None
        for landmark in self.landmarks:
            if landmark.name == landmark_name:
                target_landmark = landmark
                break
        
        if target_landmark is None:
            return False
        
        for person in self.people:
            if person.id == person_id:
                person.latitude = target_landmark.latitude
                person.longitude = target_landmark.longitude
                person.current_landmark = target_landmark
                person.target_landmark = None
                person.state = PersonState.VISITING
                person.visit_start_time = time.time()
                print(f"{person.name} телепортирован к {landmark_name}")
                return True
        
        return False
    
    def run(self, duration: int = 60, real_time: bool = True):
        """
        Запускает симуляцию
        
        Args:
            duration: продолжительность симуляции в секундах
            real_time: если True, симуляция работает в реальном времени
        """
        self.is_running = True
        start_time = time.time()
        
        print(f"\n{'='*60}")
        print("Симуляция людей в Санкт-Петербурге запущена!")
        print(f"Количество людей: {len(self.people)}")
        print(f"Достопримечательности: {[l.name for l in self.landmarks]}")
        print(f"{'='*60}\n")
        
        try:
            while self.is_running:
                elapsed = time.time() - start_time
                if elapsed >= duration:
                    break
                
                self.update()
                
                # Вывод статистики каждые 10 секунд
                if int(elapsed) % 10 == 0 and int(time.time() * 10) % 10 == 0:
                    self._print_status()
                
                if real_time:
                    time.sleep(self.interval)
            
            print(f"\n{'='*60}")
            print("Симуляция завершена!")
            self._print_final_status()
            print(f"{'='*60}\n")
        
        except KeyboardInterrupt:
            print("\nСимуляция остановлена пользователем")
            self.is_running = False
    
    def _print_status(self):
        """Выводит текущий статус симуляции"""
        walking = sum(1 for p in self.people if p.state == PersonState.WALKING)
        visiting = sum(1 for p in self.people if p.state == PersonState.VISITING)
        resting = sum(1 for p in self.people if p.state == PersonState.RESTING)
        
        avg_hr = sum(p.heart_rate for p in self.people) / len(self.people)
        
        print(f"\n[Статус] Идут: {walking}, Посещают: {visiting}, Отдыхают: {resting}")
        print(f"[Статус] Средний пульс: {avg_hr:.1f} уд/мин")
    
    def _print_final_status(self):
        """Выводит финальный статус симуляции"""
        print("\nФинальное состояние людей:")
        for person in self.people:
            status = person.to_dict()
            print(f"  {status['name']}: {status['state']} у {status['current_landmark']} "
                  f"(HR: {status['heart_rate']}, Temp: {status['temperature']})")


def main():
    """Основная функция"""
    import argparse
    
    parser = argparse.ArgumentParser(description="Симуляция людей в Санкт-Петербурге")
    parser.add_argument("-n", "--num-people", type=int, default=10,
                        help="Количество людей (по умолчанию: 10)")
    parser.add_argument("-d", "--duration", type=int, default=60,
                        help="Продолжительность симуляции в секундах (по умолчанию: 60)")
    parser.add_argument("--no-realtime", action="store_true",
                        help="Отключить режим реального времени")
    parser.add_argument("--export", type=str, default=None,
                        help="Экспортировать результат в JSON файл")
    
    args = parser.parse_args()
    
    # Создаём симуляцию
    simulation = PeopleSimulation(num_people=args.num_people)
    
    # Запускаем симуляцию
    real_time = not args.no_realtime
    simulation.run(duration=args.duration, real_time=real_time)
    
    # Экспорт в JSON если указано
    if args.export:
        data = {
            "landmarks": simulation.get_landmarks(),
            "people": simulation.get_people_status(),
            "timestamp": time.time()
        }
        with open(args.export, 'w', encoding='utf-8') as f:
            json.dump(data, f, ensure_ascii=False, indent=2)
        print(f"\nРезультаты экспортированы в {args.export}")


if __name__ == "__main__":
    main()
